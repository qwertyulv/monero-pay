package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"monero-pay/p2p"

	_ "modernc.org/sqlite"
)

type App struct {
	DB           *sql.DB
	Tracker      *SecureTracker
	Idempotency  *IdempotencyManager
	RPCClient    *RPCClient
	Oracle       *PriceOracle
	Config       Configer
	P2PPeer      *p2p.Peer
	P2PDiscovery *p2p.DiscoveryManager
}

func main() {
	cfg, err := LoadOrRunSetupWizard()
	if err != nil {
		log.Fatalf("[Fatal] Configuration setup failed: %v", err)
	}

	// Validate the effective configuration before starting any service.
	if err := validateConfig(cfg); err != nil {
		log.Fatalf("[Fatal] Invalid configuration: %v", err)
	}

	// Open the SQLite database with WAL mode and a short busy timeout.
	db, err := sql.Open("sqlite", cfg.DBPath+"?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL")
	if err != nil {
		log.Fatalf("[Fatal] Failed to connect to database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := MigrateDatabase(db); err != nil {
		log.Fatalf("[Fatal] Database migration failed: %v", err)
	}

	rpcClient := NewRPCClient(cfg.MoneroRPCURL, cfg.MoneroRPCUser, cfg.MoneroRPCPass)
	oracle := NewPriceOracle()

	// Start the price updater only when USD conversion is the selected default mode.
	// Native XMR mode can run without making background requests to external price APIs.
	if cfg.DefaultPricingMode == "usd" {
		go oracle.StartBackgroundUpdater(context.Background())
	}

	tracker := NewSecureTracker(db, rpcClient, cfg.WebhookSecret)
	idempotencyMgr := NewIdempotencyManager(db)
	p2pDiscovery := p2p.NewDiscoveryManager()

	sessionAPIKey, err := NewSessionAPIKey()
	if err != nil {
		log.Fatalf("[Fatal] Failed to generate session API key: %v", err)
	}
	log.Printf("[API] Session API key (valid only while this process is running): %s", sessionAPIKey)

	var localPeer *p2p.Peer
	if cfg.P2PEnabled {
		localPeer, err = p2p.LoadOrCreatePeer(cfg.P2PKeyPath)
		if err != nil {
			log.Fatalf("[Fatal] P2P identity initialization failed: %v", err)
		}
		log.Printf("[P2P] Peer ID: %s", localPeer.ID)
	}

	app := &App{
		DB:           db,
		Tracker:      tracker,
		Idempotency:  idempotencyMgr,
		RPCClient:    rpcClient,
		Oracle:       oracle,
		Config:       cfg,
		P2PPeer:      localPeer,
		P2PDiscovery: p2pDiscovery,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/invoices", app.handleCreateInvoice)
	mux.HandleFunc("/v1/invoices/", app.handleGetInvoice)
	mux.HandleFunc("/health", app.handleHealthCheck)
	mux.HandleFunc("/checkout/", app.handleCheckout)
	mux.HandleFunc("/checkout/api/", app.handleCheckoutAPI)
	mux.HandleFunc("/dashboard", app.handleDashboard)
	mux.HandleFunc("/dashboard/", app.handleDashboard)
	mux.HandleFunc("/api/dashboard/stats", app.handleDashboardStats)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))

	if cfg.P2PEnabled {
		mux.Handle("/p2p/v1/", p2pDiscovery.RelayHandler())
	}

	handler := publicAndAPIHandler(sessionAPIKey, mux)
	server := &http.Server{
		Addr:         cfg.ServerPort,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go app.startBackgroundTracker()
	go app.startWebhookWorker()

	log.Printf("[Startup] Monero Pay Server listening on %s", cfg.ServerPort)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("[Fatal] Server forced to shutdown: %v", err)
	}
}

func (app *App) handleCreateInvoice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	app.Idempotency.HandleCreateInvoiceWithIdempotency(w, r, func(reqBytes []byte) (interface{}, int, error) {
		return app.processCreateInvoiceBusinessLogic(reqBytes)
	})
}

func (app *App) processCreateInvoiceBusinessLogic(body []byte) (interface{}, int, error) {
	// Raw JSON values let the handler distinguish a missing field from an explicit zero.
	var req struct {
		AmountUSD  json.RawMessage `json:"amount_usd"`
		AmountXMR  json.RawMessage `json:"amount_xmr"`
		WebhookURL string          `json:"webhook_url"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return map[string]interface{}{"error": "Invalid request body; provide valid JSON."}, http.StatusBadRequest, nil
	}

	hasUSD := len(req.AmountUSD) > 0
	hasXMR := len(req.AmountXMR) > 0
	if hasUSD == hasXMR {
		return map[string]interface{}{
			"error": "Provide exactly one of amount_usd or amount_xmr.",
			"examples": map[string]interface{}{
				"usd_conversion": map[string]interface{}{"amount_usd": 10.0},
				"native_xmr":     map[string]interface{}{"amount_xmr": 0.05},
			},
		}, http.StatusBadRequest, nil
	}

	var amountUSD float64
	var rateUSD float64
	var amountAtomic uint64
	pricingMode := "xmr"

	if hasUSD {
		if err := json.Unmarshal(req.AmountUSD, &amountUSD); err != nil || amountUSD <= 0 || amountUSD > 1_000_000 {
			return map[string]string{"error": "amount_usd must be a number greater than 0 and no greater than 1000000."}, http.StatusBadRequest, nil
		}

		// USD invoices require a live or valid cached exchange rate before an amount can be locked.
		var err error
		amountAtomic, rateUSD, err = app.Oracle.ConvertUSDToAtomicXMR(amountUSD)
		if err != nil {
			log.Printf("[Oracle Error] %v", err)
			return map[string]interface{}{
				"error":      "XMR/USD market data is temporarily unavailable.",
				"message":    "The node is still running; USD pricing could not be calculated.",
				"suggestion": "Retry later, or create a native XMR invoice by sending amount_xmr instead. This skips the exchange-rate Oracle.",
				"example":    map[string]float64{"amount_xmr": 0.05},
			}, http.StatusServiceUnavailable, nil
		}
		pricingMode = "usd"
	} else {
		// Native XMR invoices bypass the price Oracle and convert directly into atomic units.
		var err error
		amountAtomic, err = ParseXMRToAtomic(string(req.AmountXMR))
		if err != nil {
			return map[string]string{"error": err.Error()}, http.StatusBadRequest, nil
		}
	}

	if amountAtomic == 0 {
		return map[string]string{"error": "calculated payment amount is zero"}, http.StatusBadRequest, nil
	}
	// SQLite INTEGER uses signed 64-bit values; reject amounts the schema cannot store safely.
	if amountAtomic > maxSQLiteInteger {
		return map[string]string{"error": "payment amount is too large for the invoice database"}, http.StatusBadRequest, nil
	}

	// Generate a real Monero subaddress through Wallet RPC; never return a placeholder address.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	subaddr, rpcIndex, rpcErr := app.RPCClient.CreateSubaddress(ctx)
	if rpcErr != nil {
		log.Printf("[Wallet Error] Failed to create subaddress: %v", rpcErr)
		return map[string]string{"error": "wallet RPC unavailable; invoice creation was refused to prevent an invalid payment address"}, http.StatusServiceUnavailable, nil
	}

	invoiceID := fmt.Sprintf("inv_%d", time.Now().UnixNano())
	now := time.Now().Unix()
	expiresAt := now + 3600
	_, err := app.DB.Exec(`
        INSERT INTO invoices (id, amount_usd, rate_usd, amount_atomic, received_atomic, overpaid_atomic,
            address, address_index, webhook_url, status, created_at, expires_at, updated_at, pricing_mode)
        VALUES (?, ?, ?, ?, 0, 0, ?, ?, ?, 'pending', ?, ?, ?, ?)`,
		invoiceID, amountUSD, rateUSD, amountAtomic, subaddr, rpcIndex, req.WebhookURL, now, expiresAt, now, pricingMode)
	if err != nil {
		log.Printf("[DB Error] Failed to save invoice: %v", err)
		return map[string]string{"error": "Internal database error"}, http.StatusInternalServerError, nil
	}

	response := map[string]interface{}{
		"invoice_id": invoiceID, "pricing_mode": pricingMode,
		"amount_usd": amountUSD, "rate_usd": rateUSD,
		"amount_atomic": amountAtomic, "amount_xmr": AtomicToXMRString(amountAtomic),
		"address": subaddr, "status": "pending",
		"expires_at": expiresAt, "created_at": now,
		"checkout_url": "/checkout/" + invoiceID,
	}
	if pricingMode == "xmr" {
		response["fiat_conversion"] = "not_requested"
	}
	return response, http.StatusCreated, nil
}

func (app *App) handleGetInvoice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	invoiceID := strings.TrimPrefix(r.URL.Path, "/v1/invoices/")
	if invoiceID == "" || strings.Contains(invoiceID, "/") {
		http.Error(w, "Invoice ID required", http.StatusBadRequest)
		return
	}

	var inv struct {
		ID             string        `json:"id"`
		AmountUSD      float64       `json:"amount_usd"`
		RateUSD        float64       `json:"rate_usd"`
		AmountAtomic   uint64        `json:"amount_atomic"`
		AmountXMR      string        `json:"amount_xmr"`
		PricingMode    string        `json:"pricing_mode"`
		ReceivedAtomic uint64        `json:"received_atomic"`
		OverpaidAtomic uint64        `json:"overpaid_atomic"`
		Address        string        `json:"address"`
		Status         InvoiceStatus `json:"status"`
		ExpiresAt      int64         `json:"expires_at"`
	}
	err := app.DB.QueryRow(`SELECT id, amount_usd, rate_usd, amount_atomic, received_atomic, overpaid_atomic, address, status, expires_at, pricing_mode FROM invoices WHERE id = ?`, invoiceID).
		Scan(&inv.ID, &inv.AmountUSD, &inv.RateUSD, &inv.AmountAtomic, &inv.ReceivedAtomic, &inv.OverpaidAtomic, &inv.Address, &inv.Status, &inv.ExpiresAt, &inv.PricingMode)
	if err == sql.ErrNoRows {
		http.Error(w, "Invoice not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	inv.AmountXMR = AtomicToXMRString(inv.AmountAtomic)
	_ = json.NewEncoder(w).Encode(inv)
}

func (app *App) handleCheckout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/checkout/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	data := struct{ InvoiceID string }{InvoiceID: id}
	tmpl, err := template.ParseFiles("./ui/checkout.html")
	if err != nil {
		http.Error(w, "Checkout unavailable", http.StatusInternalServerError)
		return
	}
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("[Checkout] render error: %v", err)
	}
}

func (app *App) handleCheckoutAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	invoiceID := strings.TrimPrefix(r.URL.Path, "/checkout/api/")
	if invoiceID == "" || strings.Contains(invoiceID, "/") {
		http.Error(w, "Invoice ID required", http.StatusBadRequest)
		return
	}
	var inv struct {
		ID             string        `json:"id"`
		AmountUSD      float64       `json:"amount_usd"`
		RateUSD        float64       `json:"rate_usd"`
		AmountAtomic   uint64        `json:"amount_atomic"`
		AmountXMR      string        `json:"amount_xmr"`
		PricingMode    string        `json:"pricing_mode"`
		ReceivedAtomic uint64        `json:"received_atomic"`
		Address        string        `json:"address"`
		Status         InvoiceStatus `json:"status"`
		ExpiresAt      int64         `json:"expires_at"`
	}
	err := app.DB.QueryRow(`SELECT id, amount_usd, rate_usd, amount_atomic, received_atomic, address, status, expires_at, pricing_mode FROM invoices WHERE id = ?`, invoiceID).Scan(&inv.ID, &inv.AmountUSD, &inv.RateUSD, &inv.AmountAtomic, &inv.ReceivedAtomic, &inv.Address, &inv.Status, &inv.ExpiresAt, &inv.PricingMode)
	if err == sql.ErrNoRows {
		http.Error(w, "Invoice not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	inv.AmountXMR = AtomicToXMRString(inv.AmountAtomic)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(inv)
}

// handleDashboard serves the merchant dashboard shell.
func (app *App) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tmpl, err := template.ParseFiles("./ui/dashboard.html")
	if err != nil {
		http.Error(w, "Dashboard unavailable", http.StatusInternalServerError)
		return
	}
	if err := tmpl.Execute(w, nil); err != nil {
		log.Printf("[Dashboard] render error: %v", err)
	}
}

// handleDashboardStats returns the protected node and invoice summary used by the dashboard.
func (app *App) handleDashboardStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	balance, unlocked, balanceErr := app.RPCClient.GetBalance()
	syncStatus, syncErr := app.RPCClient.GetSyncStatus(r.Context())
	var rate float64
	var rateErr error
	if app.Config.DefaultPricingMode == "usd" {
		rate, rateErr = app.Oracle.GetXMRUSDPrice()
	}
	rows, err := app.DB.Query(`SELECT id, amount_usd, amount_atomic, status, pricing_mode FROM invoices ORDER BY created_at DESC LIMIT 50`)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	invoices := make([]map[string]interface{}, 0, 50)
	for rows.Next() {
		var id, status, pricingMode string
		var amountUSD float64
		var amountAtomic uint64
		if err := rows.Scan(&id, &amountUSD, &amountAtomic, &status, &pricingMode); err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		invoices = append(invoices, map[string]interface{}{
			"id": id, "amount_usd": amountUSD, "amount_atomic": amountAtomic,
			"amount_xmr": AtomicToXMRString(amountAtomic), "status": status, "pricing_mode": pricingMode,
		})
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"balance": balance, "unlocked_balance": unlocked,
		"balance_error": errorString(balanceErr),
		"node_sync":     syncStatus, "node_sync_error": errorString(syncErr),
		"xmr_usd_rate": rate, "rate_error": errorString(rateErr),
		"default_pricing_mode": app.Config.DefaultPricingMode,
		"invoices":             invoices,
	}
	writeJSON(w, http.StatusOK, response)
}

// errorString converts an optional error into JSON-safe text.
func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (app *App) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"OK"}`))
}

func (app *App) startBackgroundTracker() {
	ticker := time.NewTicker(app.Config.WorkerInterval)
	defer ticker.Stop()
	app.Tracker.ProcessInvoicesCycle()
	for range ticker.C {
		app.Tracker.ProcessInvoicesCycle()
	}
}

func publicAndAPIHandler(sessionKey string, mux http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" || strings.HasPrefix(r.URL.Path, "/p2p/") || strings.HasPrefix(r.URL.Path, "/checkout/") || strings.HasPrefix(r.URL.Path, "/static/") || r.URL.Path == "/dashboard" || r.URL.Path == "/dashboard/" {
			mux.ServeHTTP(w, r)
			return
		}
		RequireSessionAPIKey(sessionKey)(mux).ServeHTTP(w, r)
	})
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
