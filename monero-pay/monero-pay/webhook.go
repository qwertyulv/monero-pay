package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"time"
)

type WebhookWorker struct {
	db     *sql.DB
	secret string
	client *http.Client
}

func NewWebhookWorker(db *sql.DB, secret string) *WebhookWorker {
	return &WebhookWorker{db: db, secret: secret, client: NewSafeHTTPClient(8 * time.Second)}
}

func (w *WebhookWorker) RunOnce() {
	rows, err := w.db.Query(`SELECT id, invoice_id, url, payload, retry_count FROM webhook_queue WHERE status='pending' AND next_retry_at <= ? ORDER BY id LIMIT 20`, time.Now().Unix())
	if err != nil {
		log.Printf("[Webhook] query failed: %v", err)
		return
	}
	defer rows.Close()
	type item struct {
		id                      int64
		invoiceID, url, payload string
		retries                 int
	}
	var items []item
	for rows.Next() {
		var x item
		if rows.Scan(&x.id, &x.invoiceID, &x.url, &x.payload, &x.retries) == nil {
			items = append(items, x)
		}
	}

	for _, x := range items {
		req, err := http.NewRequest(http.MethodPost, x.url, bytes.NewBufferString(x.payload))
		if err != nil {
			w.fail(x.id, x.retries, err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Monero-Pay-Signature", SignWebhook([]byte(x.payload), w.secret))
		req.Header.Set("X-Monero-Pay-Event", "invoice.updated")
		resp, err := w.client.Do(req)
		if err == nil {
			resp.Body.Close()
		}
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			_, _ = w.db.Exec(`UPDATE webhook_queue SET status='delivered' WHERE id=?`, x.id)
			continue
		}
		if err == nil {
			err = fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		w.fail(x.id, x.retries, err)
	}
}

func (w *WebhookWorker) fail(id int64, retries int, err error) {
	nextRetries := retries + 1
	delay := time.Duration(1<<min(nextRetries, 6)) * time.Minute
	if delay > time.Hour {
		delay = time.Hour
	}
	_, _ = w.db.Exec(`UPDATE webhook_queue SET retry_count=?, next_retry_at=?, status='pending' WHERE id=?`, nextRetries, time.Now().Add(delay).Unix(), id)
	log.Printf("[Webhook] delivery failed id=%d retry=%d: %v", id, nextRetries, err)
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (app *App) startWebhookWorker() {
	worker := NewWebhookWorker(app.DB, app.Config.WebhookSecret)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	worker.RunOnce()
	for range ticker.C {
		worker.RunOnce()
	}
}
