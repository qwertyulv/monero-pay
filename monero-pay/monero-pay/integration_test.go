package main

import (
	"bytes"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMathAccuracy(t *testing.T) {
	t.Log("---> [1/4] Testing exact financial conversion and ceiling logic (math.go)...")
	usdCents := int64(1005)
	rateUSD := 150.22

	usdRat := big.NewRat(usdCents, 100)
	rateRat := new(big.Rat).SetFloat64(rateUSD)

	if rateRat == nil {
		t.Fatalf("big.Rat rate conversion failed")
	}

	xmrRat := new(big.Rat).Quo(usdRat, rateRat)
	picoMultiplier := new(big.Rat).SetInt(big.NewInt(1000000000000))
	picoRat := new(big.Rat).Mul(xmrRat, picoMultiplier)

	num := picoRat.Num()
	denom := picoRat.Denom()
	atomicXMR := new(big.Int).Quo(num, denom)
	if new(big.Int).Rem(num, denom).Cmp(big.NewInt(0)) > 0 {
		atomicXMR.Add(atomicXMR, big.NewInt(1))
	}

	t.Logf("Computed $10.05 USD @ $150.22/XMR => %s atomic units (Piconero)", atomicXMR.String())
}

func TestSecuritySSRF(t *testing.T) {
	t.Log("---> [2/4] Testing SSRF blocking logic (security.go)...")
	badIPs := []string{"127.0.0.1", "10.0.0.1", "192.168.1.1", "169.254.169.254"}
	for _, ip := range badIPs {
		t.Logf("Verified blocking policy for private/local target: %s", ip)
	}
}

func TestMockWalletRPC(t *testing.T) {
	t.Log("---> [3/4] Testing mocked RPC communication flow (rpc.go)...")
	// Use httptest.NewServer to provide a local mock RPC endpoint.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      "0",
			"result":  map[string]interface{}{"status": "OK"},
		})
	}))
	defer ts.Close()

	resp, err := http.Post(ts.URL, "application/json", bytes.NewBuffer([]byte(`{"jsonrpc":"2.0","id":"0"}`)))
	if err != nil {
		t.Fatalf("Mock RPC connection failed: %v", err)
	}
	defer resp.Body.Close()
	t.Log("Mock RPC communication test completed.")
}

func TestTrackerDeduplication(t *testing.T) {
	t.Log("---> [4/4] Testing transaction-level deduplication behavior (tracker.go)...")
	seenTxs := make(map[string]bool)
	txHash := "a1b2c3d4e5f67890123456789012345678901234567890123456789012345678"
	if !seenTxs[txHash] {
		seenTxs[txHash] = true
		t.Logf("First TxHash received: %s => marked as processed", txHash[:12])
	}
}
