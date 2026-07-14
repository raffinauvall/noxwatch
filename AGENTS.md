# NoxWatch Agent Notes

## Mission

Build NoxWatch as a production-oriented server monitoring platform. Keep the MVP honest: completed features must run locally and planned features must be labeled planned or experimental.

## Current Phase

Phase 1 foundation is complete. Phase 2 backend is complete and its frontend flow is next:

- Go API with `/health` and `/ready`.
- PostgreSQL and Redis through Docker Compose.
- Initial PostgreSQL schema migration.
- Next.js dashboard shell with static preview data.
- README and architecture docs.
- Argon2id registration/login, rotating refresh sessions, and logout.
- Owner workspace creation and tenant-isolated reads.

Do not start enrollment until the Phase 2 frontend flow and checks pass.

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
