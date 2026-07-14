# NoxWatch

NoxWatch is a server monitoring platform for registering Linux servers with a short-lived token and monitoring health, resources, availability, and alerts from one dashboard.

Current status: Phase 2 backend. Registration, login, refresh rotation, logout, workspace creation, and workspace isolation are implemented. The auth UI, enrollment, metrics, alerts, notifications, and agent remain planned until their phase is completed.

## Architecture

- `apps/api`: Go REST API, health/readiness, config, migrations.
- `apps/web`: Next.js dashboard shell with Tailwind CSS.
- `agent`: Linux agent package area for Phase 4.
- `migrations`: PostgreSQL schema migrations.
- `deployments`: Docker and deployment assets.

## Local Setup

```bash
cp .env.example .env
docker compose up -d postgres redis
make migrate-up
make dev
```

API health:

```bash
curl http://localhost:8080/health
curl http://localhost:8080/ready
```

Auth API:

```bash
curl -i -c cookies.txt -H 'Content-Type: application/json' \
  -d '{"email":"operator@example.com","password":"change-this-password","name":"Operator"}' \
  http://localhost:8080/api/v1/auth/register
```

Web app:

```bash
npm install
make web-dev
```

## Commands

```bash
make dev
make build
make test
make lint
make migrate-up
make migrate-down
make seed
make agent-build
make agent-install-local
```

## Environment

See `.env.example`. Development credentials are only for local Docker services. Do not use them in production.

## Security Notes

The MVP will hash passwords and tokens, scope every protected resource by workspace membership, avoid logging secrets, apply request IDs, and reject cross-workspace access at the repository/service boundary.

Explicitly excluded from MVP: browser terminal, remote shell, SSH password storage, file manager, Kubernetes monitoring, billing, AI anomaly detection, and automatic remediation.
