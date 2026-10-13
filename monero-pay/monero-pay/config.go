package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Configer contains all runtime configuration required by the Monero Pay node.
type Configer struct {
	DBPath             string
	MoneroRPCURL       string
	MoneroRPCUser      string
	MoneroRPCPass      string
	WebhookSecret      string
	ServerPort         string
	WorkerInterval     time.Duration
	P2PEnabled         bool
	P2PKeyPath         string
	P2PListenAddr      string
	P2PBootstrapPeers  []string
	PrimaryAddress     string
	PrivateViewKey     string
	DefaultPricingMode string
}

// persistedConfig is the on-disk representation of Configer.
type persistedConfig struct {
	Version            int      `json:"version"`
	DBPath             string   `json:"db_path"`
	MoneroRPCURL       string   `json:"monero_rpc_url"`
	MoneroRPCUserEnc   string   `json:"monero_rpc_user_encrypted,omitempty"`
	MoneroRPCPassEnc   string   `json:"monero_rpc_pass_encrypted,omitempty"`
	WebhookSecretEnc   string   `json:"webhook_secret_encrypted,omitempty"`
	ServerPort         string   `json:"server_port"`
	WorkerInterval     string   `json:"worker_interval"`
	P2PEnabled         bool     `json:"p2p_enabled"`
	P2PKeyPath         string   `json:"p2p_key_path"`
	P2PListenAddr      string   `json:"p2p_listen_addr"`
	P2PBootstrapPeers  []string `json:"p2p_bootstrap_peers"`
	PrimaryAddress     string   `json:"monero_primary_address"`
	PrivateViewKeyEnc  string   `json:"monero_private_view_key_encrypted,omitempty"`
	DefaultPricingMode string   `json:"default_pricing_mode,omitempty"`
}

const (
	// DefaultConfigPath is the default path for the node configuration file.
	DefaultConfigPath = "./config.json"
	// DefaultConfigKeyPath stores the local encryption key used to protect sensitive configuration fields.
	DefaultConfigKeyPath = "./data/config.key"
)

// defaultConfig builds the first-run defaults shown by the setup wizard.
func defaultConfig() Configer {
	return Configer{
		DBPath:             getEnv("DB_PATH", "./monero_pay.db"),
		MoneroRPCURL:       getEnv("MONERO_RPC_URL", "http://127.0.0.1:18082"),
		MoneroRPCUser:      getEnv("MONERO_RPC_USER", ""),
		MoneroRPCPass:      getEnv("MONERO_RPC_PASS", ""),
		WebhookSecret:      getEnv("WEBHOOK_SECRET", ""),
		ServerPort:         getEnv("SERVER_PORT", ":8080"),
		WorkerInterval:     10 * time.Second,
		P2PEnabled:         getEnv("P2P_ENABLED", "true") == "true",
		P2PKeyPath:         getEnv("P2P_KEY_PATH", "./data/peer.key"),
		P2PListenAddr:      getEnv("P2P_LISTEN_ADDR", ""),
		P2PBootstrapPeers:  splitCSV(getEnv("P2P_BOOTSTRAP_PEERS", "")),
		PrimaryAddress:     getEnv("MONERO_PRIMARY_ADDRESS", ""),
		PrivateViewKey:     getEnv("MONERO_VIEW_KEY", ""),
		DefaultPricingMode: normalizePricingMode(getEnv("DEFAULT_PRICING_MODE", "usd")),
	}
}

// LoadOrRunSetupWizard loads config.json or launches the interactive first-run setup.
func LoadOrRunSetupWizard() (Configer, error) {
	configPath := getEnv("MONERO_PAY_CONFIG", DefaultConfigPath)
	keyPath := getEnv("MONERO_PAY_CONFIG_KEY", DefaultConfigKeyPath)

	cfg, err := loadConfigFile(configPath, keyPath)
	if err == nil {
		return applyEnvironmentOverrides(cfg), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return Configer{}, err
	}

	cfg, err = runSetupWizard(defaultConfig())
	if err != nil {
		return Configer{}, err
	}
	if err := saveConfigFile(configPath, keyPath, cfg); err != nil {
		return Configer{}, fmt.Errorf("save configuration: %w", err)
	}
	fmt.Printf("\n[✔] Configuration saved to %s.\n", configPath)
	return applyEnvironmentOverrides(cfg), nil
}

// loadConfigFile reads and decrypts the persisted configuration.
func loadConfigFile(path, keyPath string) (Configer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Configer{}, err
	}
	var stored persistedConfig
	if err := json.Unmarshal(data, &stored); err != nil {
		return Configer{}, fmt.Errorf("invalid configuration file: %w", err)
	}
	if stored.Version != 1 {
		return Configer{}, fmt.Errorf("unsupported configuration version: %d", stored.Version)
	}
	key, err := loadOrCreateConfigKey(keyPath, false)
	if err != nil {
		return Configer{}, err
	}
	user, err := decryptSecret(stored.MoneroRPCUserEnc, key)
	if err != nil {
		return Configer{}, fmt.Errorf("decrypt RPC username: %w", err)
	}
	pass, err := decryptSecret(stored.MoneroRPCPassEnc, key)
	if err != nil {
		return Configer{}, fmt.Errorf("decrypt RPC password: %w", err)
	}
	webhook, err := decryptSecret(stored.WebhookSecretEnc, key)
	if err != nil {
		return Configer{}, fmt.Errorf("decrypt webhook secret: %w", err)
	}
	viewKey, err := decryptSecret(stored.PrivateViewKeyEnc, key)
	if err != nil {
		return Configer{}, fmt.Errorf("decrypt private view key: %w", err)
	}
	return Configer{
		DBPath:             stored.DBPath,
		MoneroRPCURL:       stored.MoneroRPCURL,
		MoneroRPCUser:      user,
		MoneroRPCPass:      pass,
		WebhookSecret:      webhook,
		ServerPort:         stored.ServerPort,
		WorkerInterval:     mustParseDuration(stored.WorkerInterval),
		P2PEnabled:         stored.P2PEnabled,
		P2PKeyPath:         stored.P2PKeyPath,
		P2PListenAddr:      stored.P2PListenAddr,
		P2PBootstrapPeers:  append([]string(nil), stored.P2PBootstrapPeers...),
		PrimaryAddress:     stored.PrimaryAddress,
		PrivateViewKey:     viewKey,
		DefaultPricingMode: normalizePricingMode(defaultString(stored.DefaultPricingMode, "usd")),
	}, nil
}

// saveConfigFile encrypts secrets and writes the configuration with restrictive file permissions.
func saveConfigFile(path, keyPath string, cfg Configer) error {
	key, err := loadOrCreateConfigKey(keyPath, true)
	if err != nil {
		return err
	}
	user, err := encryptSecret(cfg.MoneroRPCUser, key)
	if err != nil {
		return err
	}
	pass, err := encryptSecret(cfg.MoneroRPCPass, key)
	if err != nil {
		return err
	}
	webhook, err := encryptSecret(cfg.WebhookSecret, key)
	if err != nil {
		return err
	}
	viewKey, err := encryptSecret(cfg.PrivateViewKey, key)
	if err != nil {
		return err
	}
	stored := persistedConfig{
		Version:            1,
		DBPath:             cfg.DBPath,
		MoneroRPCURL:       cfg.MoneroRPCURL,
		MoneroRPCUserEnc:   user,
		MoneroRPCPassEnc:   pass,
		WebhookSecretEnc:   webhook,
		ServerPort:         cfg.ServerPort,
		WorkerInterval:     cfg.WorkerInterval.String(),
		P2PEnabled:         cfg.P2PEnabled,
		P2PKeyPath:         cfg.P2PKeyPath,
		P2PListenAddr:      cfg.P2PListenAddr,
		P2PBootstrapPeers:  cfg.P2PBootstrapPeers,
		PrimaryAddress:     cfg.PrimaryAddress,
		PrivateViewKeyEnc:  viewKey,
		DefaultPricingMode: normalizePricingMode(defaultString(cfg.DefaultPricingMode, "usd")),
	}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0600)
}

// loadOrCreateConfigKey returns the 32-byte local key used for configuration encryption.
func loadOrCreateConfigKey(path string, create bool) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		key, err := hex.DecodeString(strings.TrimSpace(string(data)))
		if err != nil || len(key) != 32 {
			return nil, fmt.Errorf("invalid configuration encryption key")
		}
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) || !create {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(key)), 0600); err != nil {
		return nil, err
	}
	return key, nil
}

// encryptSecret protects a configuration secret with AES-GCM.
func encryptSecret(value string, key []byte) (string, error) {
	if value == "" {
		return "", nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(value), nil)
	encoded := append(nonce, ciphertext...)
	return "v1:" + base64.RawStdEncoding.EncodeToString(encoded), nil
}

// decryptSecret reverses encryptSecret for a stored configuration secret.
func decryptSecret(value string, key []byte) (string, error) {
	if value == "" {
		return "", nil
	}
	if !strings.HasPrefix(value, "v1:") {
		return "", fmt.Errorf("unsupported encrypted secret format")
	}
	encoded, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(value, "v1:"))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(encoded) < gcm.NonceSize() {
		return "", fmt.Errorf("encrypted secret is too short")
	}
	nonce := encoded[:gcm.NonceSize()]
	plaintext, err := gcm.Open(nil, nonce, encoded[gcm.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("authentication failed")
	}
	return string(plaintext), nil
}

// applyEnvironmentOverrides keeps advanced deployments compatible with environment-based configuration.
func applyEnvironmentOverrides(cfg Configer) Configer {
	cfg.DBPath = getEnv("DB_PATH", cfg.DBPath)
	cfg.MoneroRPCURL = getEnv("MONERO_RPC_URL", cfg.MoneroRPCURL)
	cfg.MoneroRPCUser = getEnv("MONERO_RPC_USER", cfg.MoneroRPCUser)
	cfg.MoneroRPCPass = getEnv("MONERO_RPC_PASS", cfg.MoneroRPCPass)
	cfg.WebhookSecret = getEnv("WEBHOOK_SECRET", cfg.WebhookSecret)
	cfg.ServerPort = getEnv("SERVER_PORT", cfg.ServerPort)
	cfg.P2PKeyPath = getEnv("P2P_KEY_PATH", cfg.P2PKeyPath)
	cfg.P2PListenAddr = getEnv("P2P_LISTEN_ADDR", cfg.P2PListenAddr)
	cfg.PrimaryAddress = getEnv("MONERO_PRIMARY_ADDRESS", cfg.PrimaryAddress)
	cfg.PrivateViewKey = getEnv("MONERO_VIEW_KEY", cfg.PrivateViewKey)
	cfg.DefaultPricingMode = normalizePricingMode(getEnv("DEFAULT_PRICING_MODE", cfg.DefaultPricingMode))
	if value, ok := os.LookupEnv("P2P_ENABLED"); ok {
		cfg.P2PEnabled = value == "true"
	}
	if value, ok := os.LookupEnv("P2P_BOOTSTRAP_PEERS"); ok {
		cfg.P2PBootstrapPeers = splitCSV(value)
	}
	return cfg
}

// normalizePricingMode maps supported pricing mode aliases to their canonical values.
func normalizePricingMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "xmr", "native-xmr", "native_xmr":
		return "xmr"
	default:
		return "usd"
	}
}

// defaultString returns fallback when value is empty after trimming whitespace.
func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// mustParseDuration parses a stored duration and falls back to the standard worker interval.
func mustParseDuration(value string) time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return 10 * time.Second
	}
	return d
}

// validateConfig checks the minimum runtime requirements after the wizard or file load.
func validateConfig(cfg Configer) error {
	if strings.TrimSpace(cfg.ServerPort) == "" {
		return fmt.Errorf("server port cannot be empty")
	}
	if strings.TrimSpace(cfg.MoneroRPCURL) == "" {
		return fmt.Errorf("Monero RPC URL cannot be empty")
	}
	if cfg.DefaultPricingMode != "usd" && cfg.DefaultPricingMode != "xmr" {
		return fmt.Errorf("default pricing mode must be either usd or xmr")
	}
	if cfg.PrivateViewKey != "" {
		decoded, err := hex.DecodeString(cfg.PrivateViewKey)
		if err != nil || len(decoded) != 32 {
			return fmt.Errorf("private view key must be 64 hexadecimal characters")
		}
	}
	return nil
}
