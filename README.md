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

The Docker socket is mounted read-only so infrastructure can be inspected. Access to the Docker socket is effectively elevated host access; use a dedicated host and review this risk before production deployment. Swarm users can adapt `deploy/stackhost.stack.yml`; the service is constrained to managers.

## Structure

The Go API lives in `cmd/stackhost`, the Vue interface in `web`, and deployment examples in `deploy`. See `AGENTS.md` for project conventions.

## Roadmap

Future releases may add deployment workflows, catalog integrations, backups, and reverse-proxy automation.

## License

MIT. See `LICENSE`.
