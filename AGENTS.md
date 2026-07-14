# NoxWatch Agent Notes

## Mission

Build NoxWatch as a production-oriented server monitoring platform. Keep the MVP honest: completed features must run locally and planned features must be labeled planned or experimental.

## Current Phase

The MVP phases are complete and in hardening/maintenance mode:

- Go API with health, readiness, Prometheus metrics, and structured request logs.
- PostgreSQL schema migrations, Redis readiness, retention, and Docker Compose health checks.
- Next.js dashboard backed by tenant-scoped API data and live SSE status events.
- Linux monitoring agent, systemd packaging, and protected local credentials.
- Local OpenSSH bootstrap with terminal-only password prompts and no stored SSH credentials.
- README, architecture, agent, and production operations documentation.
- Argon2id registration/login, rotating refresh sessions, and logout.
- Owner workspace creation and tenant-isolated reads.
- Login, registration, workspace onboarding, and authenticated dashboard states.
- One-time enrollment tokens, revocable agent identity, heartbeat, and backend offline checks.
- Static Linux agent, native metrics collectors, bounded queue, retry/backoff, and systemd service.
- Credential-bound, idempotent typed metrics ingestion with tenant-scoped history.
- Add Server enrollment flow, real server inventory, latest snapshots, and bounded historical charts.
- Duration-aware alert lifecycle, alert-driven server status, cooldown/deduplication, and signed webhook notification.
- Responsive alert/integration screens, filtered server inventory, live SSE status, and dark/light themes.
- Daily metric/session/token retention, idempotent development seed, Prometheus counters, and Docker health checks.

The public install command remains disabled until a signed downloadable release artifact is configured; local binary enrollment is supported. The supported MVP deployment has one API replica because rate limits are process-local.

## Architecture Rules

- Monorepo boundaries stay clear: `apps/api`, `apps/web`, `agent`, `migrations`, `deployments`, `docs`.
- Backend business logic does not live in HTTP handlers once domains are introduced.
- Workspace isolation is required for every protected resource.
- Tokens and credentials are generated with cryptographic randomness, stored hashed, and never logged.
- Agent uses outbound HTTPS only. No SSH, browser terminal, remote shell, or arbitrary command execution in the MVP.
- Use typed PostgreSQL columns for queried metrics. JSONB is only for extensible metadata.
- Prefer standard library and already-installed dependencies before adding anything.

## Local Commands

```bash
cp .env.example .env
docker compose up -d postgres redis
make migrate-up
make test
make lint
make build
make dev
```

## Phase Order

1. Foundation: config, logging, health/readiness, migrations, Docker, dashboard shell.
2. Auth and workspaces: registration, login, refresh sessions, owner workspace isolation.
3. Server enrollment: short-lived one-time tokens, agent identity, server online state.
4. Agent: Linux metrics, heartbeat, ingestion, retry/backoff, systemd packaging.
5. Dashboard: server list/detail, current metrics, historical charts, status updates.
6. Alerting and notification: alert lifecycle and signed webhook notification.
7. Hardening: authorization audit, rate limits, retention, tests, production docs.

## Quality Bar

- Run the smallest relevant checks before claiming work is done.
- Keep `.env.example` secret-free.
- Do not commit generated credentials or local database volumes.
- Use UTC in backend data and bytes for stored sizes.
- Keep frontend states explicit: loading, empty, error, offline, stale, permission denied.
- Add one focused test for non-trivial logic.
