package main

import (
	"database/sql"
	"fmt"
	"log"
)

type Migration struct {
	Version     int
	Description string
	Script      string
}

var migrations = []Migration{
	{Version: 1, Description: "Initial invoice and webhook schema", Script: `
        CREATE TABLE IF NOT EXISTS invoices (
            id TEXT PRIMARY KEY, amount_usd REAL NOT NULL, amount_atomic INTEGER NOT NULL,
            received_atomic INTEGER NOT NULL DEFAULT 0, overpaid_atomic INTEGER NOT NULL DEFAULT 0,
            address TEXT NOT NULL, address_index INTEGER NOT NULL, webhook_url TEXT DEFAULT '',
            status TEXT NOT NULL, created_at INTEGER NOT NULL, expires_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
        );
        CREATE UNIQUE INDEX IF NOT EXISTS idx_invoices_address_index ON invoices(address_index);
        CREATE INDEX IF NOT EXISTS idx_invoices_status ON invoices(status);
        CREATE TABLE IF NOT EXISTS webhook_queue (
            id INTEGER PRIMARY KEY AUTOINCREMENT, invoice_id TEXT NOT NULL, url TEXT NOT NULL,
            payload TEXT NOT NULL, status TEXT NOT NULL, retry_count INTEGER NOT NULL DEFAULT 0, next_retry_at INTEGER NOT NULL
        );
        CREATE INDEX IF NOT EXISTS idx_webhook_status_retry ON webhook_queue(status, next_retry_at);
    `},
	{Version: 2, Description: "Add transaction deduplication and idempotency", Script: `
        CREATE TABLE IF NOT EXISTS invoice_payments (
            tx_hash TEXT NOT NULL, invoice_id TEXT NOT NULL, subaddr_index INTEGER NOT NULL,
            amount_atomic INTEGER NOT NULL, confirmations INTEGER NOT NULL, updated_at INTEGER NOT NULL,
            PRIMARY KEY (tx_hash, invoice_id)
        );
        CREATE INDEX IF NOT EXISTS idx_invoice_payments_invoice ON invoice_payments(invoice_id);
        CREATE TABLE IF NOT EXISTS idempotency_keys (
            key_hash TEXT PRIMARY KEY, invoice_id TEXT NOT NULL, response_body TEXT NOT NULL, created_at INTEGER NOT NULL
        );
    `},
	{Version: 3, Description: "Add rate and production invoice fields to legacy databases", Script: `
        CREATE UNIQUE INDEX IF NOT EXISTS idx_invoices_address_index ON invoices(address_index);
    `},
	{Version: 4, Description: "Add invoice pricing mode for native XMR payments", Script: `
        CREATE INDEX IF NOT EXISTS idx_invoices_pricing_mode ON invoices(pricing_mode);
    `},
}

func MigrateDatabase(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}
	var current int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&current); err != nil {
		return err
	}
	for _, m := range migrations {
		if m.Version <= current {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if m.Version == 3 {
			var columnCount int
			if err = tx.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('invoices') WHERE name='rate_usd'`).Scan(&columnCount); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("migration %d inspect failed: %w", m.Version, err)
			}
			if columnCount == 0 {
				if _, err = tx.Exec(`ALTER TABLE invoices ADD COLUMN rate_usd REAL NOT NULL DEFAULT 0`); err != nil {
					_ = tx.Rollback()
					return fmt.Errorf("migration %d add rate_usd failed: %w", m.Version, err)
				}
			}
		}
		if m.Version == 4 {
			var columnCount int
			if err = tx.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('invoices') WHERE name='pricing_mode'`).Scan(&columnCount); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("migration %d inspect failed: %w", m.Version, err)
			}
			if columnCount == 0 {
				if _, err = tx.Exec(`ALTER TABLE invoices ADD COLUMN pricing_mode TEXT NOT NULL DEFAULT 'usd'`); err != nil {
					_ = tx.Rollback()
					return fmt.Errorf("migration %d add pricing_mode failed: %w", m.Version, err)
				}
			}
		}
		if _, err = tx.Exec(m.Script); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %d failed: %w", m.Version, err)
		}
		if _, err = tx.Exec(`INSERT INTO schema_migrations(version) VALUES (?)`, m.Version); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		log.Printf("[Migration] applied v%d: %s", m.Version, m.Description)
	}
	return nil
}
