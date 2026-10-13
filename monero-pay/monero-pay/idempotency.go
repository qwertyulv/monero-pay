package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type IdempotencyManager struct{ db *sql.DB }

func NewIdempotencyManager(db *sql.DB) *IdempotencyManager { return &IdempotencyManager{db: db} }

func (im *IdempotencyManager) HandleCreateInvoiceWithIdempotency(w http.ResponseWriter, r *http.Request, createFn func([]byte) (interface{}, int, error)) {
	bodyBytes, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		res, code, err := createFn(bodyBytes)
		if err != nil {
			http.Error(w, err.Error(), code)
			return
		}
		writeJSON(w, code, res)
		return
	}
	if len(key) > 255 {
		http.Error(w, "Idempotency-Key too long", http.StatusBadRequest)
		return
	}

	h := sha256.Sum256(append([]byte(key+"\x00"), bodyBytes...))
	keyHash := hex.EncodeToString(h[:])

	var existing string
	err = im.db.QueryRow(`SELECT response_body FROM idempotency_keys WHERE key_hash = ?`, keyHash).Scan(&existing)
	if err == nil {
		writeRawJSON(w, http.StatusOK, existing)
		return
	}
	if err != sql.ErrNoRows {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	// Reserve the key before creating the invoice so concurrent retries cannot create multiple invoices.
	// The reservation is removed when invoice creation fails.
	_, err = im.db.Exec(`INSERT INTO idempotency_keys(key_hash, invoice_id, response_body, created_at) VALUES(?, '', '', ?)`, keyHash, time.Now().Unix())
	if err != nil {
		if qerr := im.db.QueryRow(`SELECT response_body FROM idempotency_keys WHERE key_hash = ?`, keyHash).Scan(&existing); qerr == nil && existing != "" {
			writeRawJSON(w, http.StatusOK, existing)
			return
		}
		http.Error(w, "idempotency conflict", http.StatusConflict)
		return
	}

	res, code, createErr := createFn(bodyBytes)
	if createErr != nil {
		_, _ = im.db.Exec(`DELETE FROM idempotency_keys WHERE key_hash = ?`, keyHash)
		http.Error(w, createErr.Error(), code)
		return
	}
	// Do not cache validation or service failures; retries must receive the real status and may succeed later.
	if code < http.StatusOK || code >= http.StatusMultipleChoices {
		_, _ = im.db.Exec(`DELETE FROM idempotency_keys WHERE key_hash = ?`, keyHash)
		writeJSON(w, code, res)
		return
	}
	resBytes, err := json.Marshal(res)
	if err != nil {
		_, _ = im.db.Exec(`DELETE FROM idempotency_keys WHERE key_hash = ?`, keyHash)
		http.Error(w, "response serialization error", http.StatusInternalServerError)
		return
	}

	invoiceID := ""
	if m, ok := res.(map[string]interface{}); ok {
		if v, ok := m["invoice_id"].(string); ok {
			invoiceID = v
		}
	}
	if _, err = im.db.Exec(`UPDATE idempotency_keys SET invoice_id = ?, response_body = ? WHERE key_hash = ?`, invoiceID, string(resBytes), keyHash); err != nil {
		http.Error(w, "failed to persist idempotency result", http.StatusInternalServerError)
		return
	}
	writeRawJSON(w, code, string(resBytes))
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	b, _ := json.Marshal(v)
	writeRawJSON(w, code, string(b))
}
func writeRawJSON(w http.ResponseWriter, code int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = fmt.Fprint(w, body)
}
