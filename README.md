# StackHost

StackHost is an early-stage, self-hosted control panel for Docker and Docker Swarm. It provides first-admin onboarding, cookie sessions, SQLite persistence, projects, applications, Docker inventory, and an infrastructure-aware dashboard in Portuguese (Brasil).

## Local development

Requirements: Go 1.25+, Node.js 22+, npm. Docker is optional; the UI explains when it is unavailable.

```bash
cp .env.example .env
make setup
make dev
```

Open http://localhost:8080. A clean database redirects users to `/setup` in the frontend flow. Run `make test`, `make lint`, and `make build` for checks.

For frontend hot reload during development, run `cd web && npm run dev` in a second terminal. The Vite server proxies the UI only; the Go server remains responsible for the API.

When Docker is connected without Swarm, open **Infraestrutura** and use **Preparar ambiente**. The operation uses the Docker Engine SDK, requires an administrator session, records an audit event, and never removes existing containers, images, or volumes. Docker socket access is intentionally explicit because it grants elevated access to the host.

The initial read-only inventory APIs are `/api/v1/infrastructure/containers`, `/images`, `/volumes`, and `/networks`; each accepts `limit` and returns bounded DTOs.

## Docker

```bash
cp .env.example .env
docker compose up --build
```

The Docker socket is mounted into the container so StackHost can inspect and operate workloads. A Unix socket remains effectively elevated host access even when the application filesystem is read-only; use a dedicated host and review this risk before production deployment. Swarm users can adapt `deploy/stackhost.stack.yml`; the service is constrained to managers.

For a production-like install, set strong `STACKHOST_SESSION_SECRET` and `STACKHOST_ENCRYPTION_KEY` values, keep `STACKHOST_COOKIE_SECURE=true` behind HTTPS, and enable ingress only after DNS points to the host. The optional ACME certificate cache lives under `STACKHOST_CERT_STORAGE`.

Operational endpoints include asynchronous deployments under `/api/v1/applications/:id/deployments`, local runtime metrics under `/api/v1/applications/:id/metrics`, system backups under `/api/v1/backups/system`, and administrator-only webhook notifications under `/api/v1/notifications`. Backups currently use local storage; S3-compatible destinations and scheduled retention are not enabled yet.

## Structure

The Go API lives in `cmd/stackhost`, the Vue interface in `web`, and deployment examples in `deploy`. See `AGENTS.md` for project conventions.

## Roadmap

The current branch includes asynchronous deploy history, native ingress/domains, runtime actions and logs, local health/metrics, local backups, and webhook notifications. Remaining follow-up work includes global Swarm metrics, S3/scheduled backups, richer catalog workflows, and installation automation.

## License

MIT. See `LICENSE`.
