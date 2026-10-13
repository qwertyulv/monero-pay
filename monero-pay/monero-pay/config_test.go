package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestConfigEncryptionRoundTrip verifies that sensitive configuration values round-trip through encrypted storage.
func TestConfigEncryptionRoundTrip(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	keyPath := filepath.Join(dir, "data", "config.key")

	original := Configer{
		DBPath:             filepath.Join(dir, "monero_pay.db"),
		MoneroRPCURL:       "http://127.0.0.1:18082",
		MoneroRPCUser:      "rpc-user",
		MoneroRPCPass:      "rpc-pass",
		WebhookSecret:      "webhook-secret",
		ServerPort:         ":8080",
		WorkerInterval:     10 * time.Second,
		P2PEnabled:         true,
		P2PKeyPath:         filepath.Join(dir, "data", "peer.key"),
		P2PListenAddr:      "127.0.0.1:28080",
		P2PBootstrapPeers:  []string{"https://seed.example/p2p"},
		PrimaryAddress:     "4primary-address",
		PrivateViewKey:     strings.Repeat("ab", 32),
		DefaultPricingMode: "xmr",
	}

	if err := saveConfigFile(configPath, keyPath, original); err != nil {
		t.Fatalf("saveConfigFile failed: %v", err)
	}

	stored, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config failed: %v", err)
	}
	text := string(stored)
	for _, secret := range []string{original.MoneroRPCUser, original.MoneroRPCPass, original.WebhookSecret, original.PrivateViewKey} {
		if strings.Contains(text, secret) {
			t.Fatalf("plaintext secret found in config.json: %q", secret)
		}
	}

	loaded, err := loadConfigFile(configPath, keyPath)
	if err != nil {
		t.Fatalf("loadConfigFile failed: %v", err)
	}
	if loaded.MoneroRPCUser != original.MoneroRPCUser || loaded.MoneroRPCPass != original.MoneroRPCPass ||
		loaded.WebhookSecret != original.WebhookSecret || loaded.PrivateViewKey != original.PrivateViewKey {
		t.Fatalf("sensitive configuration did not round-trip correctly")
	}
	if loaded.ServerPort != original.ServerPort || loaded.P2PListenAddr != original.P2PListenAddr ||
		loaded.DefaultPricingMode != "xmr" ||
		len(loaded.P2PBootstrapPeers) != 1 || loaded.P2PBootstrapPeers[0] != original.P2PBootstrapPeers[0] {
		t.Fatalf("non-sensitive configuration did not round-trip correctly")
	}
}

func TestValidateConfigAcceptsPricingModes(t *testing.T) {
	cfg := defaultConfig()
	cfg.DefaultPricingMode = "xmr"
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("native XMR mode should be valid: %v", err)
	}
	cfg.DefaultPricingMode = "invalid"
	if err := validateConfig(cfg); err == nil {
		t.Fatal("invalid pricing mode should be rejected")
	}
}
