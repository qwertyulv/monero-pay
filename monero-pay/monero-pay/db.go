package main

// InvoiceStatus is the persisted lifecycle state of an invoice.
type InvoiceStatus string

const (
	StatusPending              InvoiceStatus = "pending"
	StatusPartialPaid          InvoiceStatus = "partial_paid"
	StatusPaidUnconfirmed      InvoiceStatus = "paid_unconfirmed"
	StatusConfirmed            InvoiceStatus = "confirmed"
	StatusExpired              InvoiceStatus = "expired"
	StatusLatePayment          InvoiceStatus = "late_payment"
	StatusLatePaymentConfirmed InvoiceStatus = "late_payment_confirmed"
)

type Invoice struct {
	ID             string        `json:"id"`
	AmountUSD      float64       `json:"amount_usd"`
	RateUSD        float64       `json:"rate_usd"`
	PricingMode    string        `json:"pricing_mode"`
	AmountAtomic   uint64        `json:"amount_atomic"`
	ReceivedAtomic uint64        `json:"received_atomic"`
	OverpaidAtomic uint64        `json:"overpaid_atomic"`
	Address        string        `json:"address"`
	AddressIndex   uint64        `json:"address_index"`
	Status         InvoiceStatus `json:"status"`
	WebhookURL     string        `json:"webhook_url"`
	CreatedAt      int64         `json:"created_at"`
	ExpiresAt      int64         `json:"expires_at"`
	UpdatedAt      int64         `json:"updated_at"`
}
