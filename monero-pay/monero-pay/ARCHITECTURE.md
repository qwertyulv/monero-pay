# Monero Pay Module Maintenance Map

## Core payment layer

- `main.go`: process entry point, HTTP routes, component wiring, and background workers.
- `config.go`: persistent configuration loading, encrypted secret storage, environment overrides, and configuration validation.
- `setup_wizard.go`: first-run interactive CLI configuration wizard.
- `db.go`: invoice state constants and persisted invoice model.
- `db_migration.go`: ordered SQLite schema migrations.
- `rpc.go`: Monero Wallet RPC client and transfer/sync queries.
- `oracle.go`: XMR/USD provider aggregation, caching, and conversion entry point.
- `math.go`: exact atomic-unit conversion using rational arithmetic.
- `tracker.go`: incoming transaction detection, deduplication, confirmations, expiration, and invoice state transitions.
- `idempotency.go`: duplicate invoice-request protection.
- `security.go`: API compatibility alias, HMAC signing, and SSRF-safe outbound HTTP.
- `webhook.go`: durable webhook queue, delivery, and retry logic.
- `session_api.go`: process-lifetime API credential generation and authentication.

## P2P protocol layer

- `p2p/peer.go`: persistent Ed25519 node identity.
- `p2p/bootstrap.go`: bootstrap/relay peer discovery.
- `p2p/offer.go`: signed public offer envelopes and matching metadata.
- `p2p/discovery.go`: offer verification, TTL, deduplication, and relay HTTP handlers.
- `p2p/transport.go`: future end-to-end encrypted peer transport boundary.

## User interface

- `ui/checkout.html`: public buyer checkout page with Monero URI and local QR rendering.
- `ui/dashboard.html`: merchant dashboard using the current session API key.
- `static/qrcode.min.js`: vendored QRCode.js runtime with no CDN dependency.

## Configuration lifecycle

1. First launch without `config.json` starts `setup_wizard.go`.
2. Sensitive fields are encrypted into `config.json` with AES-GCM.
3. The local encryption key is stored in `data/config.key` with restrictive permissions.
4. The P2P identity is stored separately in `data/peer.key` and remains stable across restarts.
5. The HTTP API key is generated in memory at every process start and is never persisted.

## Future 1% platform fee

Do not put fee accounting into `tracker.go`. Add an independent `fee.go` and `settlement.go`, and bind the fee policy to the invoice at creation time.
