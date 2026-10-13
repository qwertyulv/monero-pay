package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

// runSetupWizard collects the minimum configuration required for a first-time node launch.
func runSetupWizard(cfg Configer) (Configer, error) {
	if !isInteractiveTerminal() {
		return Configer{}, fmt.Errorf("no configuration file exists and stdin is not interactive; run the node from a terminal once to complete setup")
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Println("==================================================")
	fmt.Println("   Monero Pay Node - First Run Setup")
	fmt.Println("==================================================")
	fmt.Println()
	fmt.Println("[?] No configuration file was found. Please complete the initial setup.")
	fmt.Println()

	cfg.PrimaryAddress = promptRequired(reader, "1. Monero primary receiving address (stored with node configuration)", cfg.PrimaryAddress)
	cfg.PrivateViewKey = promptSecret(reader, "2. Monero private view key (optional; reserved for future local wallet derivation)", cfg.PrivateViewKey)
	cfg.MoneroRPCURL = promptString(reader, "3. Monero wallet RPC URL", cfg.MoneroRPCURL)
	cfg.MoneroRPCUser = promptString(reader, "4. Monero RPC username (optional)", cfg.MoneroRPCUser)
	cfg.MoneroRPCPass = promptSecret(reader, "5. Monero RPC password (optional)", cfg.MoneroRPCPass)
	cfg.ServerPort = promptString(reader, "6. HTTP service port", cfg.ServerPort)
	cfg.DBPath = promptString(reader, "7. Database path", cfg.DBPath)
	cfg.WebhookSecret = promptSecret(reader, "8. Webhook signing secret (optional)", cfg.WebhookSecret)
	cfg.DefaultPricingMode = promptPricingMode(reader, cfg.DefaultPricingMode)

	defaultP2P := "yes"
	if !cfg.P2PEnabled {
		defaultP2P = "no"
	}
	cfg.P2PEnabled = promptBool(reader, "10. Enable P2P discovery/relay", defaultP2P)
	cfg.P2PKeyPath = promptString(reader, "11. Persistent P2P identity path", cfg.P2PKeyPath)
	cfg.P2PListenAddr = promptString(reader, "12. P2P listen address (optional for now)", cfg.P2PListenAddr)
	cfg.P2PBootstrapPeers = promptCSV(reader, "13. P2P bootstrap peers, comma-separated (optional)", cfg.P2PBootstrapPeers)

	cfg.WorkerInterval = 10 * time.Second
	if err := validateConfig(cfg); err != nil {
		return Configer{}, err
	}

	fmt.Println()
	fmt.Println("[✔] Setup validation completed.")
	fmt.Println("[i] Sensitive fields will be encrypted in config.json using a local 32-byte key in data/config.key.")
	return cfg, nil
}

// isInteractiveTerminal detects whether the current process can safely run the setup wizard.
func isInteractiveTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// promptString reads a value and returns the default when the user presses Enter.
func promptString(reader *bufio.Reader, label, defaultValue string) string {
	if defaultValue != "" {
		fmt.Printf("[?] %s [%s]: ", label, defaultValue)
	} else {
		fmt.Printf("[?] %s: ", label)
	}
	value := readLine(reader)
	if value == "" {
		return defaultValue
	}
	return value
}

// promptRequired reads a non-empty value and repeats until the user supplies one.
func promptRequired(reader *bufio.Reader, label, defaultValue string) string {
	for {
		value := promptString(reader, label, defaultValue)
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
		fmt.Println("[!] This value is required.")
	}
}

// promptSecret reads a sensitive value without echo suppression to avoid platform-specific terminal dependencies.
func promptSecret(reader *bufio.Reader, label, defaultValue string) string {
	if defaultValue != "" {
		fmt.Printf("[?] %s [stored value kept]: ", label)
	} else {
		fmt.Printf("[?] %s: ", label)
	}
	value := readLine(reader)
	if value == "" {
		return defaultValue
	}
	return strings.TrimSpace(value)
}

// promptBool reads a yes/no answer using a human-readable default.
func promptBool(reader *bufio.Reader, label, defaultValue string) bool {
	for {
		fmt.Printf("[?] %s [%s]: ", label, defaultValue)
		value := strings.ToLower(strings.TrimSpace(readLine(reader)))
		if value == "" {
			value = defaultValue
		}
		switch value {
		case "y", "yes", "true", "1":
			return true
		case "n", "no", "false", "0":
			return false
		default:
			fmt.Println("[!] Please answer yes or no.")
		}
	}
}

// promptPricingMode asks which pricing mode the dashboard should use by default.
// API clients may still choose amount_usd or amount_xmr independently per invoice.
func promptPricingMode(reader *bufio.Reader, defaultValue string) string {
	defaultValue = normalizePricingMode(defaultValue)
	for {
		fmt.Printf("[?] 9. Default Dashboard pricing mode (API requests can override per invoice) [usd/xmr, default %s]: ", defaultValue)
		value := strings.ToLower(strings.TrimSpace(readLine(reader)))
		if value == "" {
			return defaultValue
		}
		switch value {
		case "usd", "usd-conversion", "usd_conversion":
			return "usd"
		case "xmr", "native-xmr", "native_xmr":
			return "xmr"
		default:
			fmt.Println("[!] Choose usd or xmr.")
		}
	}
}

// promptCSV reads an optional comma-separated list.
func promptCSV(reader *bufio.Reader, label string, defaults []string) []string {
	defaultValue := strings.Join(defaults, ",")
	value := promptString(reader, label, defaultValue)
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return splitCSV(value)
}

// readLine reads one line and strips the trailing newline characters.
func readLine(reader *bufio.Reader) string {
	value, err := reader.ReadString('\n')
	if err != nil && len(value) == 0 {
		return ""
	}
	return strings.TrimSpace(value)
}
