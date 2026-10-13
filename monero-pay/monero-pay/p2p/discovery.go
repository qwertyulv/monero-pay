package p2p

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type DiscoveryManager struct {
	mu         sync.RWMutex
	seenOffers map[string]time.Time
	offers     map[string]OfferBlob
	peers      map[string]string
	MaxOffers  int
	OfferTTL   time.Duration
}

func NewDiscoveryManager() *DiscoveryManager {
	return &DiscoveryManager{seenOffers: make(map[string]time.Time), offers: make(map[string]OfferBlob), peers: make(map[string]string), MaxOffers: 5000, OfferTTL: 24 * time.Hour}
}

func (d *DiscoveryManager) RegisterPeer(peerID, addr string) {
	if peerID == "" || addr == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.peers[peerID] = addr
}

func (d *DiscoveryManager) PeerAddresses() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]string, 0, len(d.peers))
	for _, addr := range d.peers {
		out = append(out, addr)
	}
	return out
}

func (d *DiscoveryManager) HandleIncomingOffer(blob OfferBlob) (bool, error) {
	now := time.Now()
	if !blob.Verify(now) {
		return false, fmt.Errorf("invalid or expired offer")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cleanupLocked(now)
	if _, ok := d.seenOffers[blob.OfferID]; ok {
		return false, nil
	}
	if len(d.offers) >= d.MaxOffers {
		return false, fmt.Errorf("offer cache full")
	}
	d.seenOffers[blob.OfferID] = now
	d.offers[blob.OfferID] = blob
	return true, nil
}

func (d *DiscoveryManager) ListOffers() []OfferBlob {
	now := time.Now()
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cleanupLocked(now)
	out := make([]OfferBlob, 0, len(d.offers))
	for _, o := range d.offers {
		out = append(out, o)
	}
	return out
}

func (d *DiscoveryManager) cleanupLocked(now time.Time) {
	for id, o := range d.offers {
		if now.Unix() >= o.ExpiresAt {
			delete(d.offers, id)
		}
	}
	for id, seenAt := range d.seenOffers {
		if now.Sub(seenAt) > d.OfferTTL {
			delete(d.seenOffers, id)
		}
	}
}

// RelayHandler exposes only signed public discovery envelopes. It is not a
// custody endpoint and does not accept private keys or payment secrets.
func (d *DiscoveryManager) RelayHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/p2p/v1/offers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"offers": d.ListOffers()})
		case http.MethodPost:
			r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
			var blob OfferBlob
			if err := json.NewDecoder(r.Body).Decode(&blob); err != nil {
				http.Error(w, "invalid offer", http.StatusBadRequest)
				return
			}
			added, err := d.HandleIncomingOffer(blob)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			status := http.StatusOK
			if !added {
				status = http.StatusConflict
			}
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]bool{"accepted": added})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/p2p/v1/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
		var req struct {
			PeerID     string `json:"peer_id"`
			ListenAddr string `json:"listen_addr"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PeerID == "" || strings.TrimSpace(req.ListenAddr) == "" {
			http.Error(w, "invalid bootstrap request", http.StatusBadRequest)
			return
		}
		d.RegisterPeer(req.PeerID, req.ListenAddr)
		peers := d.PeerAddresses()
		filtered := peers[:0]
		for _, p := range peers {
			if p != req.ListenAddr {
				filtered = append(filtered, p)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(BootstrapResponse{Peers: filtered})
	})
	return mux
}
