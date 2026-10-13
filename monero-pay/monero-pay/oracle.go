package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type PriceOracle struct {
	mu          sync.RWMutex
	cachedPrice float64
	lastUpdated time.Time
	cacheTTL    time.Duration
	httpClient  *http.Client
}

func NewPriceOracle() *PriceOracle {
	oracle := &PriceOracle{
		cacheTTL:   3 * time.Minute,
		httpClient: NewSafeHTTPClient(3 * time.Second),
	}
	return oracle
}

// StartBackgroundUpdater refreshes the price in the background after the service starts.
// Refresh the price every minute so normal API calls can use the in-memory cache.
func (p *PriceOracle) StartBackgroundUpdater(ctx context.Context) {
	// Refresh immediately so the cache is populated at startup.
	p.refreshPrice()

	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				p.refreshPrice()
			case <-ctx.Done():
				return
			}
		}
	}()
}

// GetXMRUSDPrice returns a cached price or performs a synchronous refresh when needed.
func (p *PriceOracle) GetXMRUSDPrice() (float64, error) {
	p.mu.RLock()
	// Return immediately while the cache is still within its TTL.
	if time.Since(p.lastUpdated) < p.cacheTTL && p.cachedPrice > 0 {
		price := p.cachedPrice
		p.mu.RUnlock()
		return price, nil
	}
	p.mu.RUnlock()

	// Refresh the cache when the current value is stale.
	return p.refreshPrice()
}

// refreshPrice queries all configured providers concurrently and accepts the first valid result.
func (p *PriceOracle) refreshPrice() (float64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Re-check the cache after acquiring the write lock.
	if time.Since(p.lastUpdated) < p.cacheTTL && p.cachedPrice > 0 {
		return p.cachedPrice, nil
	}

	providers := []func() (float64, error){
		p.fetchCoinGecko,
		p.fetchKraken,
		p.fetchBinance,
		p.fetchCoinCap,
	}

	// Request all providers concurrently and keep the first valid price.
	type result struct {
		price float64
		err   error
	}
	ch := make(chan result, len(providers))

	for _, fetch := range providers {
		go func(f func() (float64, error)) {
			price, err := f()
			ch <- result{price: price, err: err}
		}(fetch)
	}

	var newPrice float64
	for i := 0; i < len(providers); i++ {
		res := <-ch
		if res.err == nil && res.price > 0 {
			newPrice = res.price
			break // Stop as soon as a valid provider response is available.
		}
	}

	if newPrice > 0 {
		p.cachedPrice = newPrice
		p.lastUpdated = time.Now()
		return newPrice, nil
	}

	// Use a stale cache for up to 24 hours when all live providers are temporarily unavailable.
	if p.cachedPrice > 0 && time.Since(p.lastUpdated) < 24*time.Hour {
		log.Printf("[Oracle Warning] All price APIs down. Using fallback stale cache: $%.2f", p.cachedPrice)
		return p.cachedPrice, nil
	}

	// Refuse to create a price when every provider and the stale cache are unavailable.
	return 0, errors.New("critical: all exchange rate sources failed")
}

func (p *PriceOracle) fetchCoinGecko() (float64, error) {
	resp, err := p.httpClient.Get("https://api.coingecko.com/api/v3/simple/price?ids=monero&vs_currencies=usd")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var res map[string]map[string]float64
	if err := json.NewDecoder(resp.Body).Decode(&res); err == nil {
		if val, ok := res["monero"]["usd"]; ok {
			return val, nil
		}
	}
	return 0, errors.New("coingecko failed")
}

func (p *PriceOracle) fetchKraken() (float64, error) {
	resp, err := p.httpClient.Get("https://api.kraken.com/0/public/Ticker?pair=XMRUSD")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var res struct {
		Result struct {
			XXMRZUSD struct {
				C []string `json:"c"`
			} `json:"XXMRZUSD"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err == nil && len(res.Result.XXMRZUSD.C) > 0 {
		return strconv.ParseFloat(res.Result.XXMRZUSD.C[0], 64)
	}
	return 0, errors.New("kraken failed")
}

func (p *PriceOracle) fetchBinance() (float64, error) {
	resp, err := p.httpClient.Get("https://api.binance.com/api/v3/ticker/price?symbol=XMRUSDT")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var res struct {
		Price string `json:"price"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err == nil {
		return strconv.ParseFloat(res.Price, 64)
	}
	return 0, errors.New("binance failed")
}

func (p *PriceOracle) fetchCoinCap() (float64, error) {
	resp, err := p.httpClient.Get("https://api.coincap.io/v2/rates/monero")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var res struct {
		Data struct {
			RateUsd string `json:"rateUsd"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err == nil {
		return strconv.ParseFloat(res.Data.RateUsd, 64)
	}
	return 0, errors.New("coincap failed")
}

func (p *PriceOracle) ConvertUSDToAtomicXMR(usd float64) (uint64, float64, error) {
	price, err := p.GetXMRUSDPrice()
	if err != nil {
		return 0, 0, err
	}
	usdCentsFloat := usd * 100
	if usdCentsFloat > float64(^uint64(0)>>1) {
		return 0, 0, fmt.Errorf("USD amount too large")
	}
	usdCents := int64(usdCentsFloat + 0.0000000001)
	if usdCents <= 0 {
		return 0, 0, errors.New("USD amount rounds below one cent")
	}
	atomicAmount, err := ConvertUSDToAtomic(usdCents, price)
	if err != nil {
		return 0, 0, err
	}
	return atomicAmount, price, nil
}
