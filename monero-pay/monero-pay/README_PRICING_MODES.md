# Invoice Pricing Modes

Monero Pay supports two invoice amount formats. Every invoice creation request must supply exactly one of `amount_usd` or `amount_xmr`.

## USD conversion

Request:

```json
{
  "amount_usd": 10.0
}
```

The node obtains an XMR/USD rate, locks the XMR target amount into the invoice, and returns `pricing_mode: "usd"` plus `rate_usd`.

If live providers and the acceptable stale cache are unavailable, the endpoint returns HTTP `503` with an actionable JSON response. The service remains running; the caller can retry later or create a native XMR invoice.

## Native XMR

Request:

```json
{
  "amount_xmr": 0.05
}
```

The node converts the decimal amount to atomic units using integer arithmetic. This path bypasses the exchange-rate Oracle completely and returns `pricing_mode: "xmr"`, `rate_usd: 0`, and `fiat_conversion: "not_requested"`.

The amount must use plain decimal notation with no more than 12 decimal places. Scientific notation is rejected. The upper bound is constrained to SQLite's signed 64-bit integer range. Since a native XMR invoice has no USD notional for the existing risk-tier rule, the tracker conservatively requires 10 confirmations for native XMR invoices.

## Setup Wizard default

During first run, the wizard asks whether the Dashboard should default to USD conversion (`usd`) or native XMR (`xmr`). This setting controls the Dashboard's quick-invoice button and whether the background price updater runs. API clients still select the mode per invoice using the request field. Thus `amount_xmr` always bypasses Oracle, regardless of the configured default.

Native XMR pricing does not require Internet access to obtain a price quote. It still requires a running, reachable Monero Wallet RPC because a fresh, real subaddress must be generated for every invoice. This is intentional: the node never substitutes a synthetic address.

## API examples

```bash
# USD-denominated invoice
curl -X POST http://127.0.0.1:8080/v1/invoices \
  -H 'Authorization: Bearer YOUR_SESSION_API_KEY' \
  -H 'Content-Type: application/json' \
  -d '{"amount_usd":10.0}'

# Native XMR invoice; no exchange-rate lookup
curl -X POST http://127.0.0.1:8080/v1/invoices \
  -H 'Authorization: Bearer YOUR_SESSION_API_KEY' \
  -H 'Content-Type: application/json' \
  -d '{"amount_xmr":0.05}'
```

## Exchange-rate unavailable response

```json
{
  "error": "XMR/USD market data is temporarily unavailable.",
  "message": "The node is still running; USD pricing could not be calculated.",
  "suggestion": "Retry later, or create a native XMR invoice by sending amount_xmr instead. This skips the exchange-rate Oracle.",
  "example": { "amount_xmr": 0.05 }
}
```
