# StackHost

StackHost is an early-stage, self-hosted control panel for Docker and Docker Swarm. It provides first-admin onboarding, cookie sessions, SQLite persistence, projects, and an infrastructure-aware dashboard.

## Local development

Requirements: Go 1.23+, Node.js 22+, npm. Docker is optional; the UI explains when it is unavailable.

```bash
cp .env.example .env
make setup
make dev
```

Open http://localhost:8080. A clean database redirects users to `/setup` in the frontend flow. Run `make test`, `make lint`, and `make build` for checks.

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
