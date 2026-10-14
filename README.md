# Monero Pay

#**No censorship No KYC Self-hosted information**

An open-source, self-hosted Monero payment gateway written in Go.

Monero Pay provides invoice management, hosted checkout pages, payment tracking, merchant API endpoints, webhook delivery, and a peer-to-peer discovery layer.

## Features

- **Hosted checkout:** Payment pages with Monero payment URIs and QR codes.
- **Invoice API:** Create and query payment invoices.
- **Monero Wallet RPC integration:** Generate real Monero subaddresses for invoices.
- **Payment tracking:** Track incoming transfers, confirmations, partial payments, and overpayments.
- **Flexible pricing:** Support native XMR invoices and USD-denominated invoices.
- **Webhook delivery:** Queue and retry webhook notifications.
- **Merchant dashboard:** View payment-related information through a browser interface.
- **SQLite storage:** Persist invoices and payment records locally.
- **P2P discovery foundation:** Signed public offers and peer discovery/relay components.
- **Cross-platform builds:** Windows, macOS, and Linux releases.

Some P2P transport capabilities are not implemented yet. See [README_P2P.md](README_P2P.md)https://github.com/qwertyulv/monero-pay/blob/main/monero-pay/monero-pay/README_P2P.md for the current scope and limitations.

## Requirements

- A running Monero Wallet RPC service (`monero-wallet-rpc`).
- A Monero daemon or other suitable node configuration for the wallet RPC service.
- Network access to the configured Wallet RPC endpoint.
- A terminal for first-run interactive configuration.

Monero Pay does not bundle the Monero blockchain or Wallet RPC software.

## Quick Start

1. Download the archive matching your operating system from [GitHub Releases](https://github.com/qwertyulv/monero-pay/releases).
2. Extract the archive.
3. Open a terminal in the extracted directory.
4. Run the executable.

On Linux or macOS:

```bash
chmod +x monero-pay
./monero-pay
```

On Windows, run `monero-pay.exe` from a terminal.

On first launch, the setup wizard requests the required configuration and creates `config.json` and local data files.

Keep the configuration encryption key and persistent P2P identity safe. Do not commit runtime configuration or wallet secrets to source control.

## Configuration

The first-run wizard configures:

- Monero Wallet RPC URL and optional credentials
- Primary Monero receiving address
- HTTP service port
- SQLite database path
- Webhook signing secret
- Default invoice pricing mode
- P2P discovery settings

Configuration can also be adjusted using the supported environment variables. See [README_SETUP.md][(README_SETUP.md)]https://github.com/qwertyulv/monero-pay/blob/main/monero-pay/monero-pay/README_SETUP.md and `config.example.json`.

## API

The main endpoints include:

| Endpoint | Purpose |
| --- | --- |
| `GET /health` | Health check |
| `POST /v1/invoices` | Create an invoice |
| `GET /v1/invoices/{id}` | Retrieve invoice details |
| `GET /checkout/{id}` | Hosted checkout page |
| `GET /dashboard` | Merchant dashboard |

Merchant API requests require the current process-lifetime API key. See [README_SESSION_API.md][(README_SESSION_API.md)](https://github.com/qwertyulv/monero-pay/blob/main/monero-pay/monero-pay/README_SESSION_API.md) for authentication details.

### Create a native XMR invoice

```bash
curl -X POST http://127.0.0.1:8080/v1/invoices \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_SESSION_API_KEY" \
  -d '{"amount_xmr": 0.05}'
```

Replace `YOUR_SESSION_API_KEY` with the key printed by the running process.

Native XMR invoices do not require an exchange-rate quote, but invoice creation still requires a working Wallet RPC service.

For USD-denominated invoices, send `amount_usd` instead. Each request must specify exactly one of `amount_usd` or `amount_xmr`.

## Security Notes

- Keep Wallet RPC private and restrict access to trusted clients.
- Do not expose sensitive RPC endpoints directly to the public internet.
- Use HTTPS when serving the API over an untrusted network.
- Back up the SQLite database and persistent application data securely.
- Protect `data/config.key`, `data/peer.key`, and configuration files.
- The merchant API key changes whenever the process restarts.
- Test payment confirmation and webhook behavior before handling real funds.

The local encryption of configuration values is not a substitute for secure host access controls or secret management.

## Documentation

- [Setup Guide](README_SETUP.md)https://github.com/qwertyulv/monero-pay/blob/main/monero-pay/monero-pay/README_SETUP.md
- [Session API Authentication](README_SESSION_API.md)https://github.com/qwertyulv/monero-pay/blob/main/monero-pay/monero-pay/README_SESSION_API.md
- [P2P Architecture and Limitations](README_P2P.md)https://github.com/qwertyulv/monero-pay/blob/main/monero-pay/monero-pay/README_P2P.md
- [Pricing Modes](README_PRICING_MODES.md)https://github.com/qwertyulv/monero-pay/blob/main/monero-pay/monero-pay/README_PRICING_MODES.md
- [Architecture Overview](ARCHITECTURE.md)https://github.com/qwertyulv/monero-pay/blob/main/monero-pay/monero-pay/ARCHITECTURE.md

## Development

Run tests:

```bash
go test ./...
```

Build locally:

```bash
go build -o monero-pay .
```

Cross-platform release builds are published through GitHub Actions when a version tag is pushed.

**If you have any good suggestions, please email me:qwertyulv@protonmail.com**

## License

MIT. See [LICENSE](LICENSE) for the full license text.
