# Monero Pay Node

Monero Pay is a self-hosted Monero payment gateway with a P2P discovery layer, public hosted checkout, merchant API, invoice tracking, webhook delivery, and a process-lifetime session API key.

## First launch

Start the binary from an interactive terminal. When `config.json` is missing, the CLI Setup Wizard asks for the node and wallet settings and writes the configuration automatically.

Sensitive configuration fields are encrypted in `config.json` with AES-GCM. The local encryption key is stored in `data/config.key`. The persistent P2P identity remains in `data/peer.key`.

## Runtime API key

A new API key is generated on every process start. It is valid only while that node process remains alive. Copy the key from the terminal into the Dashboard or use it in `Authorization: Bearer` / `X-API-Key` headers.

## Payment safety

Invoice creation requires the Monero Wallet RPC to create a real payment subaddress. The node never creates a synthetic fallback address.

USD-denominated invoices require a valid exchange rate; native XMR invoices (`{"amount_xmr": 0.05}`) skip the Oracle entirely. When market data is unavailable, USD requests receive a helpful HTTP 503 response with a native-XMR example instead of a vague error. Native XMR invoices still require the Monero Wallet RPC to create a real subaddress.

The first-run wizard asks for a default invoice pricing mode (`usd` or `xmr`). That preference controls the Dashboard quick-invoice button and whether the background price updater runs. API clients choose their per-invoice mode by sending exactly one of `amount_usd` or `amount_xmr`.

## Delivery contents

The clean source archive intentionally excludes local SQLite databases, WAL files, P2P private keys, configuration keys, and IDE workspace files. Unit and integration test source files remain in the repository.

See `README_SETUP.md`, `README_SESSION_API.md`, `README_P2P.md`, and `README_PRICING_MODES.md` for operating details.
