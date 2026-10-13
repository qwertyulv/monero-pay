# First-Run CLI Setup Wizard

When `config.json` is missing, the node starts an interactive setup wizard in the terminal. The wizard is designed for users who do not want to manage environment variables manually.

## What the wizard collects

- Monero primary receiving address
- Private view key
- Monero Wallet RPC URL
- Optional RPC username/password
- HTTP service port
- SQLite database path
- Optional webhook signing secret
- Default invoice pricing mode: USD conversion (`usd`) or native XMR (`xmr`)
- P2P enablement
- Persistent P2P identity path
- Optional P2P listen address
- Optional bootstrap peer list

## Secret storage

`config.json` does not store the private view key, RPC password, RPC username, or webhook secret in plaintext. Those fields are encrypted with AES-GCM. The local encryption key is stored in `data/config.key` and should never be committed to source control.

This is local at-rest encryption, not an operating-system keychain. Anyone who can read both `config.json` and `data/config.key` can decrypt the stored secrets.

## Runtime API key

The merchant API uses a process-lifetime session key. The key is printed once at startup, lives only in memory, and becomes invalid when the process exits. Starting the node again creates a different key.

The Dashboard lets the operator paste the current key into the browser tab. The key is stored only in `sessionStorage` for that tab.

## Wallet-address safety

Invoice creation now fails when the Monero Wallet RPC cannot create a valid subaddress. The node no longer generates synthetic fallback addresses. This prevents an unavailable wallet backend from producing a payment destination that is not controlled by the merchant.

## Exchange-rate safety

If all live price providers fail and no cache newer than 24 hours exists, USD invoice creation fails rather than using a development/mock exchange rate. Native XMR invoices can still be created without a rate quote.


## Pricing mode and offline operation

The wizard's pricing mode is the Dashboard default. API callers choose an amount per invoice by sending exactly one of `amount_usd` or `amount_xmr`. Native XMR requests skip all price APIs, and the background Oracle updater is not started when the default mode is `xmr`. A running Monero Wallet RPC is still required to generate a real subaddress. See `README_PRICING_MODES.md` for API examples and the exchange-rate failure response.
