package main

import (
	"bytes"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"
)

// MockRoundTripper intercepts HTTP requests and returns deterministic test responses.
type MockRoundTripper struct {
	Handler func(req *http.Request) (*http.Response, error)
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.Handler(req)
}

// mockResponse constructs a minimal HTTP response for tests.
func mockResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(bytes.NewBufferString(body)),
		Header:     make(http.Header),
	}
}

// 1. Test provider outage handling and stale-cache fallback.
func TestOracle_AllAPIsDown_WithFallback(t *testing.T) {
	oracle := NewPriceOracle()

	// Inject a mock transport that makes every provider return HTTP 500.
	oracle.httpClient.Transport = &MockRoundTripper{
		Handler: func(req *http.Request) (*http.Response, error) {
			return mockResponse(http.StatusInternalServerError, "Internal Server Error"), nil
		},
	}

	// 1.1 With no cache and all providers down, the oracle must return a critical error.
	_, err := oracle.GetXMRUSDPrice()
	if err == nil || err.Error() != "critical: all exchange rate sources failed" {
		t.Fatalf("expected critical error, got: %v", err)
	}

	// 1.2 A one-hour-old cache remains valid for stale fallback.
	oracle.cachedPrice = 165.50
	oracle.lastUpdated = time.Now().Add(-1 * time.Hour)

	price, err := oracle.GetXMRUSDPrice()
	if err != nil {
		t.Fatalf("expected fallback to stale cache, got error: %v", err)
	}
	if price != 165.50 {
		t.Fatalf("expected stale price 165.50, got: %f", price)
	}

	// 1.3 A cache older than 24 hours must not be used.
	oracle.lastUpdated = time.Now().Add(-25 * time.Hour)
	_, err = oracle.GetXMRUSDPrice()
	if err == nil {
		t.Fatalf("expected error for cache older than 24h, but succeeded")
	}
}

// 2. Test provider failover.
func TestOracle_FailoverChain(t *testing.T) {
	oracle := NewPriceOracle()

	// Make the first three providers fail and the fourth provider succeed.
	oracle.httpClient.Transport = &MockRoundTripper{
		Handler: func(req *http.Request) (*http.Response, error) {
			switch req.URL.Host {
			case "api.coingecko.com":
				return mockResponse(http.StatusInternalServerError, "down"), nil
			case "api.kraken.com":
				return mockResponse(http.StatusTooManyRequests, "rate limited"), nil
			case "api.binance.com":
				return mockResponse(http.StatusOK, `{"price": "0.0"}`), nil // Provider returns an unusable zero price.
			case "api.coincap.io":
				return mockResponse(http.StatusOK, `{"data":{"rateUsd":"155.25"}}`), nil
			default:
				return mockResponse(http.StatusNotFound, ""), nil
			}
		},
	}

	price, err := oracle.GetXMRUSDPrice()
	if err != nil {
		t.Fatalf("failover failed: %v", err)
	}
	if price != 155.25 {
		t.Fatalf("expected price 155.25 from CoinCap, got: %f", price)
	}
}

// 3. Test conversion edge cases.
func TestOracle_ConvertUSDToAtomicXMR_EdgeCases(t *testing.T) {
	oracle := NewPriceOracle()
	oracle.cachedPrice = 100.0 // 1 XMR = 100 USD
	oracle.lastUpdated = time.Now()

	// 3.1 Reject amounts below one cent.
	_, _, err := oracle.ConvertUSDToAtomicXMR(0.001)
	if err == nil || err.Error() != "USD amount rounds below one cent" {
		t.Fatalf("expected below 1 cent error, got: %v", err)
	}

	// 3.2 Reject negative amounts.
	_, _, err = oracle.ConvertUSDToAtomicXMR(-10.0)
	if err == nil {
		t.Fatalf("expected error for negative USD amount, got nil")
	}

	// 3.3 Verify a normal conversion at an exact 1 XMR result.
	atomic, p, err := oracle.ConvertUSDToAtomicXMR(100.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p != 100.0 || atomic != 1000000000000 {
		t.Fatalf("unexpected atomic calculation: got %d atomic units", atomic)
	}
}

// 4. Test concurrent access for race safety.
func TestOracle_Concurrency(t *testing.T) {
	oracle := NewPriceOracle()
	oracle.httpClient.Transport = &MockRoundTripper{
		Handler: func(req *http.Request) (*http.Response, error) {
			return mockResponse(http.StatusOK, `{"monero":{"usd":160.0}}`), nil
		},
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = oracle.GetXMRUSDPrice()
			_, _, _ = oracle.ConvertUSDToAtomicXMR(50.0)
		}()
	}
	wg.Wait()
}
