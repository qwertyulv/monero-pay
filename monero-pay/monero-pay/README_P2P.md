# P2P Protocol Layer - Phase 1

This phase adds the basic P2P Discovery / Relay skeleton to Monero Pay without pretending that direct encrypted transport is already complete.

## Modules

- `p2p/peer.go`: persistent Ed25519 node identity; the Peer ID remains stable across restarts.
- `p2p/offer.go`: signed public matching offers using integer atomic XMR and fiat cents instead of float64 amounts.
- `p2p/bootstrap.go`: register with bootstrap/relay nodes and retrieve known peer addresses.
- `p2p/discovery.go`: signature verification, TTL enforcement, offer deduplication, in-memory offer cache, and relay HTTP API.
- `p2p/transport.go`: boundary for the future end-to-end encrypted peer session; private trade transport is not implemented yet.

## Network boundary

`/p2p/v1/*` is a public discovery/relay interface and does not use the merchant session API key.

It accepts signed offers and peer addresses only. It does not accept private keys, custody XMR, or decide payment outcomes.

## Why offers are not fully encrypted

The matching stage needs a minimal public metadata set such as trade direction, XMR amount, fiat amount, currency, and payment type. Without that data, an unknown peer cannot discover a compatible trade. Private identity and payment details should move into the future authenticated end-to-end transport session after a match.

## Configuration

- `P2P_ENABLED=true` enables the discovery/relay layer.
- `P2P_KEY_PATH=./data/peer.key` stores the persistent node identity.
- `P2P_LISTEN_ADDR=` is reserved for a future direct transport listener.
- `P2P_BOOTSTRAP_PEERS=` contains a comma-separated list of bootstrap URLs.

The first start creates the persistent peer key. Do not commit it to source control.

## Not implemented yet

- QUIC/TCP direct transport
- End-to-end encrypted peer sessions
- Full Gossip neighbor forwarding
- Persistent peer/offer database storage
- Relay rate limiting, Sybil resistance, and reputation

This phase is a protocol foundation, not a completed fully decentralized trading network.
