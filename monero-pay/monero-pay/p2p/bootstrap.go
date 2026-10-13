package p2p

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type BootstrapClient struct {
	SeedNodes []string
	Client    *http.Client
}

type BootstrapResponse struct {
	Peers []string `json:"peers"`
}

func (b *BootstrapClient) RegisterToSeed(ctx context.Context, myPeer *Peer, listenAddr string) ([]string, error) {
	if myPeer == nil {
		return nil, fmt.Errorf("peer is nil")
	}
	client := b.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	var lastErr error
	for _, seed := range b.SeedNodes {
		seed = strings.TrimRight(strings.TrimSpace(seed), "/")
		if seed == "" {
			continue
		}
		body, _ := json.Marshal(map[string]string{"peer_id": myPeer.ID, "listen_addr": listenAddr})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, seed+"/p2p/v1/bootstrap", bytes.NewReader(body))
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		var result BootstrapResponse
		decErr := json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if decErr != nil {
			lastErr = decErr
			continue
		}
		if resp.StatusCode/100 != 2 {
			lastErr = fmt.Errorf("bootstrap returned %s", resp.Status)
			continue
		}
		return result.Peers, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no bootstrap seed configured")
}
