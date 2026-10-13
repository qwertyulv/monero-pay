# Session API Key

The merchant API uses a process-lifetime session key instead of a persistent API key.

## Lifecycle

1. Every process start generates a new cryptographically random 32-byte key using `crypto/rand`.
2. The key is held only in process memory and printed to the node terminal at startup.
3. When the node process exits, the key disappears and the old credential stops working.
4. The next start generates a different credential.

The session key is never written to SQLite, `config.json`, `data/config.key`, or the P2P identity file.

## Authentication

Use either: 

```http
Authorization: Bearer mp_xxxxxxxxxxxxxxxxx
```

or:

```http
X-API-Key: mp_xxxxxxxxxxxxxxxxx
```

Merchant endpoints under `/v1/` and the protected dashboard statistics endpoint require the current session key.

## Public endpoints

The following endpoints are intentionally public so buyers and P2P peers do not need merchant credentials:

- `/health`
- `/p2p/v1/*`
- `/checkout/*`
- `/checkout/api/*`
- `/static/*`
- `/dashboard`

The dashboard page itself is public, but it asks the operator to paste the current session key before requesting protected data.

## Remote use

When the node is exposed beyond a trusted LAN, put TLS/HTTPS in front of it. The session key is a bearer credential and must not be sent over plaintext HTTP across an untrusted network.
