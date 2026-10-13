package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"time"
)

type SecureTracker struct {
	db         *sql.DB
	rpcClient  *RPCClient
	webhookKey string
}

func NewSecureTracker(db *sql.DB, rpc *RPCClient, webhookKey string) *SecureTracker {
	return &SecureTracker{db: db, rpcClient: rpc, webhookKey: webhookKey}
}

func (st *SecureTracker) ProcessInvoicesCycle() {
	transfers, err := st.rpcClient.GetAllIncomingTransfers()
	if err != nil {
		log.Printf("[Tracker Warning] Wallet RPC unavailable: %v", err)
		return
	}

	tx, err := st.db.Begin()
	if err != nil {
		log.Printf("[Tracker] DB begin: %v", err)
		return
	}
	defer tx.Rollback()
	now := time.Now().Unix()

	for _, tr := range transfers {
		var existingConf uint64
		err := tx.QueryRow(`SELECT confirmations FROM invoice_payments WHERE tx_hash = ? AND invoice_id = (SELECT id FROM invoices WHERE address_index = ?)`, tr.TxID, tr.SubaddrIndex).Scan(&existingConf)
		if err == sql.ErrNoRows {
			var invID string
			if err := tx.QueryRow(`SELECT id FROM invoices WHERE address_index = ?`, tr.SubaddrIndex).Scan(&invID); err == nil {
				if _, err := tx.Exec(`INSERT OR IGNORE INTO invoice_payments(tx_hash, invoice_id, subaddr_index, amount_atomic, confirmations, updated_at) VALUES(?,?,?,?,?,?)`, tr.TxID, invID, tr.SubaddrIndex, tr.Amount, tr.Confirmations, now); err != nil {
					log.Printf("[Tracker] payment insert failed: %v", err)
				}
			}
		} else if err == nil && existingConf != tr.Confirmations {
			_, _ = tx.Exec(`UPDATE invoice_payments SET confirmations = ?, updated_at = ? WHERE tx_hash = ? AND invoice_id = (SELECT id FROM invoices WHERE address_index = ?)`, tr.Confirmations, now, tr.TxID, tr.SubaddrIndex)
		}
	}

	rows, err := tx.Query(`SELECT id, amount_atomic, received_atomic, address_index, status, amount_usd, webhook_url, expires_at, pricing_mode FROM invoices WHERE status IN ('pending','partial_paid','paid_unconfirmed','expired','late_payment')`)
	if err != nil {
		log.Printf("[Tracker] invoice query: %v", err)
		return
	}
	defer rows.Close()

	type update struct {
		id                 string
		status             InvoiceStatus
		received, overpaid uint64
		webhookURL         string
		pricingMode        string
		amount             uint64
		amountUSD          float64
		addressIndex       uint64
	}
	var updates []update

	for rows.Next() {
		var inv Invoice
		if err := rows.Scan(&inv.ID, &inv.AmountAtomic, &inv.ReceivedAtomic, &inv.AddressIndex, &inv.Status, &inv.AmountUSD, &inv.WebhookURL, &inv.ExpiresAt, &inv.PricingMode); err != nil {
			continue
		}

		var sumReceived, minConfs uint64
		minConfs = ^uint64(0)
		var txCount int
		pRows, err := tx.Query(`SELECT amount_atomic, confirmations FROM invoice_payments WHERE invoice_id = ?`, inv.ID)
		if err != nil {
			continue
		}
		for pRows.Next() {
			var amt, conf uint64
			if pRows.Scan(&amt, &conf) == nil {
				sumReceived += amt
				if conf < minConfs {
					minConfs = conf
				}
				txCount++
			}
		}
		pRows.Close()

		newStatus := inv.Status
		overpaid := uint64(0)
		isExpired := inv.Status == StatusExpired || inv.Status == StatusLatePayment

		// Expire unpaid invoices. Payments discovered after expiry are handled as late payments.
		if txCount == 0 && inv.Status == StatusPending && now >= inv.ExpiresAt {
			newStatus = StatusExpired
		} else if txCount > 0 {
			// Allow a 0.5% underpayment tolerance using integer arithmetic.
			acceptable := inv.AmountAtomic * 995 / 1000
			if sumReceived >= acceptable {
				reqConfs := uint64(10)
				// Native XMR invoices have no USD notional, so use the conservative confirmation threshold.
				if inv.PricingMode != "xmr" && inv.AmountUSD < 100 {
					reqConfs = 1
				}
				if sumReceived > inv.AmountAtomic {
					overpaid = sumReceived - inv.AmountAtomic
				}
				if minConfs >= reqConfs {
					if isExpired {
						newStatus = StatusLatePaymentConfirmed
					} else if overpaid > 0 && overpaid*100 > inv.AmountAtomic*5 {
						newStatus = InvoiceStatus("paid_overpaid")
					} else {
						newStatus = StatusConfirmed
					}
				} else if isExpired {
					newStatus = StatusLatePayment
				} else {
					newStatus = StatusPaidUnconfirmed
				}
			} else if sumReceived > 0 {
				newStatus = StatusPartialPaid
			}
		}

		if newStatus != inv.Status || sumReceived != inv.ReceivedAtomic {
			updates = append(updates, update{id: inv.ID, status: newStatus, received: sumReceived, overpaid: overpaid, webhookURL: inv.WebhookURL, pricingMode: inv.PricingMode, amount: inv.AmountAtomic, amountUSD: inv.AmountUSD, addressIndex: inv.AddressIndex})
		}
	}
	if err := rows.Err(); err != nil {
		log.Printf("[Tracker] rows: %v", err)
		return
	}

	for _, u := range updates {
		if _, err := tx.Exec(`UPDATE invoices SET status=?, received_atomic=?, overpaid_atomic=?, updated_at=? WHERE id=?`, u.status, u.received, u.overpaid, now, u.id); err != nil {
			log.Printf("[Tracker] update %s: %v", u.id, err)
			return
		}
		if u.webhookURL != "" && u.status != StatusPending {
			payload, _ := json.Marshal(map[string]interface{}{
				"event": "invoice.updated", "invoice_id": u.id, "status": u.status,
				"amount_atomic": u.amount, "received_atomic": u.received, "overpaid_atomic": u.overpaid,
				"amount_usd": u.amountUSD, "pricing_mode": u.pricingMode, "updated_at": now,
			})
			if _, err := tx.Exec(`INSERT INTO webhook_queue(invoice_id,url,payload,status,retry_count,next_retry_at) VALUES(?,?,?,'pending',0,?)`, u.id, u.webhookURL, string(payload), now); err != nil {
				log.Printf("[Webhook] queue insert failed for %s: %v", u.id, err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		log.Printf("[Tracker] commit: %v", err)
	}
}
