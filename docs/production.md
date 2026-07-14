# Production Deployment

NoxWatch is production-oriented, but the MVP deployment shape is intentionally one API replica, one web service, PostgreSQL, and Redis behind a TLS reverse proxy. Do not expose PostgreSQL or Redis publicly.

## Required Controls

- Set `APP_ENV=production`.
- Generate a unique high-entropy `AUTH_SECRET` through a secret manager. Rotating it invalidates access tokens and existing encrypted webhook configuration.
- Use PostgreSQL TLS; startup rejects production URLs containing `sslmode=disable`.
- Set HTTPS `PUBLIC_WEB_URL`, HTTPS `NEXT_PUBLIC_API_URL`, and an exact `CORS_ALLOWED_ORIGINS` allowlist.
- Terminate TLS with Caddy, Nginx, a cloud load balancer, or an ingress. Preserve streaming and disable proxy buffering for the SSE route.
- Keep database backups, test restores, and monitor `/ready`, `/metrics`, container health, disk capacity, and retention job logs.
- Run migrations as a separate release step before switching application traffic.

Example secret generation:

```bash
openssl rand -base64 48
```

## Container Release

Build immutable images from a tagged commit:

```bash
docker compose build api web
docker compose run --rm api /app/api -migrate up
docker compose up -d
docker compose ps
```

The Compose file is a local/single-host baseline, not a high-availability PostgreSQL or Redis deployment. Replace its development database password and avoid publishing dependency ports in production.

## Data Lifecycle

Raw `metric_samples` and cascading disk/network rows are deleted daily after `METRIC_RETENTION_DAYS` (default 30). Expired/revoked sessions and completed enrollment tokens older than seven days are also removed. Historical API queries are limited to 30 days and 2,000 points.

Hourly/daily aggregate tables are not implemented in the MVP. Add rollups before offering query ranges beyond raw retention.

## Scaling Boundary

The database schema, bounded queries, idempotent ingestion, and SSE payloads support hundreds of servers at 30-60 second intervals. The current auth and ingestion rate limiters are process-local, so run one API replica. Before horizontal API scaling:

1. Move rate counters to Redis.
2. Add a shared status-event broker or PostgreSQL/Redis pub-sub fanout.
3. Add metric table partitioning and hourly rollups based on measured volume.
4. Dispatch webhook deliveries through a durable background queue with retries and a dead-letter policy.

## Incident Response

- Revoke a user session by logging out or revoking its database session record.
- Revoke an agent from the server detail controls or `DELETE /api/v1/servers/:serverId/agent`.
- Rotate a compromised webhook by deleting and recreating the channel; its signing secret is only returned at creation.
- Treat leaked enrollment tokens as short-lived but revoke them immediately through the enrollment-token endpoint.
- Audit sensitive changes in `audit_logs`; application logs intentionally exclude credential values and payload bodies.

## Dependency Review

Run before releases:

```bash
npm audit --omit=dev
go list -m -u all
make test
make lint
make build
```

As of the current lockfile, npm reports two moderate PostCSS findings inherited from the latest stable Next.js package. Do not apply the proposed forced Next.js downgrade. Re-evaluate when a patched stable Next.js release is available.
