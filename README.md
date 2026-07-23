# NoxWatch

NoxWatch is a workspace-scoped Linux server monitoring platform. Enroll an outbound-only agent with a 15-minute one-time token, then monitor heartbeat, CPU, memory, swap, disks, network traffic, uptime, historical metrics, and alert incidents from one dashboard.

## MVP Status

The MVP is implemented and runnable locally:

- Argon2id registration/login, rotating refresh sessions, logout, and immediate session revocation.
- Owner workspaces with tenant-isolated servers, metrics, alert rules, events, and integrations.
- Hashed, revocable, single-use enrollment tokens and hashed permanent agent credentials.
- Static Go Linux agent using `/proc`, `/sys`, and `statfs`; no inbound port, SSH, or command execution.
- Heartbeat, backend offline detection, typed idempotent metric ingestion, 30-day raw retention, and SSE status updates.
- Responsive server inventory, latest metrics, bounded historical charts, maintenance mode, agent revocation, and server deletion.
- Duration-aware warning/critical alerts with firing/resolved lifecycle, cooldown, deduplication, and signed webhooks.
- Structured logs, request IDs, health/readiness endpoints, Prometheus-compatible `/metrics`, Docker health checks, and graceful shutdown.

The public `curl | bash` installer and Docker agent remain **planned** until signed downloadable release artifacts exist. Local static-binary and systemd installation is supported now.

## Repository

```text
apps/api/       Go REST API and domain services
apps/web/       Next.js App Router dashboard
agent/          Linux monitoring agent
deployments/    Dockerfiles, systemd unit, installation scripts
docs/           Architecture, agent, and production notes
migrations/     Transactional PostgreSQL migrations
```

## Prerequisites

- Go 1.26+
- Node.js 24+ for parity with the web container
- Docker with Compose
- Linux for running the monitoring agent

## Quick Start

```bash
cp .env.example .env
npm install
docker compose up -d postgres redis
make migrate-up
make seed
```

Run API and web in separate terminals:

```bash
make api-dev
make web-dev
```

Open `http://localhost:3000`. Seed login:

- Email: `demo@noxwatch.local`
- Password: the local `SEED_DEMO_PASSWORD` value

After signing in, open the dashboard's **Guide** tab for first-time setup, enrollment, daily tunnel operation, security notes, and troubleshooting commands.

Seed execution is rejected unless `APP_ENV=development`. It is idempotent and creates three clearly simulated servers with 24 hours of typed metric history and a sample alert.

To run the complete container stack instead:

```bash
docker compose up --build
```

The API does not auto-migrate. Run `make migrate-up` before starting it against a fresh database.

## Commands

```bash
make dev                 # Build and run the complete Compose stack
make api-dev             # Run the Go API on the host
make web-dev             # Run Next.js on the host
make ssh-tunnel SSH_TARGET=user@host  # Link a remote agent to the local API
make local-helper        # Enable Add Server's Open in terminal button
make local-helper-install # Start the helper automatically after desktop login
make migrate-up          # Apply pending migrations
make migrate-down        # Roll back the latest migration
make seed                # Idempotent development-only demo data
make test                # API integration, agent, and frontend tests
make lint                # Go formatting and frontend ESLint
make build               # API, static agent, and production web build
make agent-build         # dist/noxwatch-agent
make agent-install-local # Install binary, config, and systemd unit as root
```

Integration tests use `TEST_DATABASE_URL`. They skip when it is unset and run against the configured test database when `.env` is loaded by `make`.

## Agent Enrollment

1. Sign in, create a workspace, and choose **Add Server**.
2. Select **SSH bootstrap** and enter the Linux SSH target and API endpoint reachable over the LAN.
3. Build the agent and run the generated command from the repository root:

```bash
make agent-build
```

OpenSSH asks for the SSH password in the local terminal; NoxWatch never receives or stores it. Manual binary installation remains available through `deployments/scripts/install-local.sh`.

To launch SSH bootstrap directly from Add Server, keep the local helper running in another terminal:

```bash
make local-helper
```

The helper binds only to `127.0.0.1:9734`, accepts requests only from `PUBLIC_WEB_URL`, validates every bootstrap field, and opens the existing bootstrap script in a local terminal. When the Agent API endpoint is loopback (the default `http://127.0.0.1:18082`), the SSH tunnel moves to the background after enrollment and the terminal closes automatically. The dashboard can then start or stop all saved tunnels. Profiles contain only the SSH target and ports; passwords remain terminal-only and are never stored.

For the dashboard controls to work after a laptop restart, install the helper as a user service once:

```bash
make local-helper-install
```

It starts after desktop login. **Start all tunnels** opens one terminal, prompts only where OpenSSH needs authentication, starts each tunnel in the background, and closes after success.

For an already-enrolled server outside the local network, reconnect its reverse SSH tunnel with:

```bash
make ssh-tunnel SSH_TARGET=deploy@203.0.113.10
```

Then enter `http://127.0.0.1:18082` as the reachable API endpoint during SSH bootstrap. On later starts, use **Start all tunnels**; the saved agent endpoint does not change. See [docs/agent.md](docs/agent.md#reverse-ssh-tunnel).

The enrollment token is stored only as a SHA-256 hash, expires after 15 minutes, and is returned once. After enrollment, the agent removes the token file and atomically writes its permanent credential with mode `0600`. See [docs/agent.md](docs/agent.md).

## API Examples

Register and retain the refresh cookie:

```bash
curl -i -c cookies.txt -H 'Content-Type: application/json' \
  -d '{"email":"operator@example.com","password":"use-a-long-local-password","name":"Operator"}' \
  http://localhost:8080/api/v1/auth/register
```

Protected endpoints use `Authorization: Bearer <access-token>`. Important routes include:

- `POST /api/v1/servers/enrollment-token`
- `POST /api/v1/agent/enroll`
- `POST /api/v1/agent/heartbeat`
- `POST /api/v1/agent/metrics`
- `GET /api/v1/servers/:serverId/metrics`
- `GET /api/v1/workspaces/:workspaceId/events` (SSE)
- `GET|POST /api/v1/alert-rules`
- `GET|POST /api/v1/notification-channels`

Full route ownership and flows are documented in [docs/architecture.md](docs/architecture.md).

## Configuration

| Variable | Purpose |
| --- | --- |
| `APP_ENV` | `development` or `production`; production enables secure cookies and TLS checks. |
| `DATABASE_URL` | PostgreSQL connection URL. Production must not use `sslmode=disable`. |
| `TEST_DATABASE_URL` | PostgreSQL URL used by integration tests. |
| `REDIS_ADDR` | Redis address used by readiness checks and reserved for distributed runtime coordination. |
| `AUTH_SECRET` | At least 32 characters; signs access tokens and derives webhook encryption/signing keys. |
| `PUBLIC_WEB_URL` | Dashboard base URL included in notifications; HTTPS is required in production. |
| `NEXT_PUBLIC_API_URL` | Browser-visible API URL, embedded during the web image build. |
| `CORS_ALLOWED_ORIGINS` | Comma-separated browser origin allowlist. |
| `METRIC_RETENTION_DAYS` | Raw metric retention, from 7 to 365 days; defaults to 30. |
| `SEED_DEMO_PASSWORD` | Development seed password; seed refuses production mode. |

`API_PORT`, `WEB_PORT`, `POSTGRES_PORT`, and `REDIS_PORT` only control host-side Compose port mappings.

## Security

- Passwords use Argon2id; refresh, enrollment, and agent credentials are never stored plaintext.
- Every protected query joins workspace membership or binds an agent credential to one server.
- Request bodies are capped at 1 MiB; metric collections and historical ranges are bounded.
- Cookie refresh is SameSite Strict and origin-checked; CORS is allowlisted.
- Webhook URLs are AES-GCM encrypted, redirects are constrained, production blocks private/link-local destinations, and payloads are HMAC-SHA256 signed.
- Sensitive operations write audit records. Logs omit passwords, tokens, credentials, authorization headers, and metric payload bodies.

`npm audit --omit=dev` currently reports two moderate findings from PostCSS 8.4.31 pinned by Next.js 16.2.10. Next 16.2.10 is the latest stable release available to this project; `npm audit fix --force` proposes an unsafe major downgrade, so the advisory is tracked rather than force-patched.

## Troubleshooting

- `/ready` returns 503: verify PostgreSQL and Redis health with `docker compose ps`.
- Port already in use: set `API_PORT`, `WEB_PORT`, `POSTGRES_PORT`, or `REDIS_PORT` in `.env`.
- Browser cannot refresh: ensure its exact origin is in `CORS_ALLOWED_ORIGINS`.
- Agent rejects HTTP: use HTTPS, or set `allow_insecure_http: true` only for local development.
- SSH bootstrap cannot connect: verify the SSH target and remote `sudo` access; use a LAN API address or start the reverse tunnel before using its loopback endpoint.
- Reverse SSH tunnel cannot bind: confirm remote SSH forwarding is enabled and `REMOTE_API_PORT` is unused.
- No metrics: run `sudo noxwatch-agent status` and inspect `journalctl -u noxwatch-agent`.
- Migration path error: run commands through the Makefile or set `MIGRATIONS_DIR` correctly for the process working directory.

Production deployment and scaling constraints are in [docs/production.md](docs/production.md).

## Excluded From MVP

Browser terminals, remote shell/commands, SSH password storage, file management, full log aggregation, Kubernetes, Windows agents, billing, public status pages, AI anomaly detection, and automatic remediation are intentionally not implemented. The broader future roadmap remains documented in [docs/architecture.md](docs/architecture.md).
