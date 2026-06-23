# mytonstorage-backend VPS deploy

**[Russian version](README.ru.md)**

Stack (hub compose):
- `backend` (image from registry)
- `tonutils-storage` (image from registry)
- `postgres` (official image)

## Docker Hub (recommended for VPS)

| Step | Where | Command |
|------|-------|---------|
| 1 | Dev machine | `BACKEND_IMAGE=... TONUTILS_STORAGE_IMAGE=... task image:build:push` |
| 2 | VPS | Clone repo, `deploy/` + `deploy/secrets/agents-ca.crt` |
| 3 | VPS | `task hub:init` → edit `deploy/.env.hub` |
| 4 | VPS | `task hub:up` |

Hub compose ([docker-compose.hub.yml](docker-compose.hub.yml)) uses `pull_policy: always` and skips local `build:`.

### Dev machine: build + push

```bash
BACKEND_IMAGE=<user>/mytonstorage-backend:latest \
TONUTILS_STORAGE_IMAGE=<user>/mytonutils-storage:v1.5.1 \
task image:build:push
```

Release backend (no debug CORS): omit `BUILD_TAGS` or `BUILD_TAGS=`.

Separate steps: `task image:build:backend`, `task image:build:tonutils-storage`, `task image:push`.

### VPS: pull + up

```bash
task hub:init
nano deploy/.env.hub
task hub:up
task hub:ps
task hub:health
```

Required in `.env.hub`:
- `BACKEND_IMAGE`, `TONUTILS_STORAGE_IMAGE`
- `SYSTEM_HOST` — public frontend domain (TON Connect proof)
- `AGENT_ENDPOINTS`, `AGENT_AUTH_TOKEN`, `AGENT_CA_CERT_FILE` — same as mytonprovider coordinator
- `TONUTILS_STORAGE_EXTERNAL_IP` — host public IPv4 or DNS (overlay DHT)
- `POSTGRES_PORT` — avoid host conflicts (e.g. `5433` if `5432` is taken)

Place coordinator agent CA at `deploy/secrets/agents-ca.crt` — see [secrets/README.md](secrets/README.md).

Stop: `task hub:down`.

### Task reference (hub)

| Task | Description |
|------|-------------|
| `image:build:backend` | Build backend image (`BACKEND_IMAGE`) |
| `image:build:tonutils-storage` | Build storage daemon image (`TONUTILS_STORAGE_IMAGE`) |
| `image:build` / `image:push` / `image:build:push` | Build and/or push both |
| `hub:init` | Create `.env.hub`, generate `SYSTEM_PRIVATE_KEY` if placeholder |
| `hub:up` / `down` / `ps` / `logs` / `hub:health` | Hub stack lifecycle |

## Local dev (build on host)

Uses [docker-compose.yml](docker-compose.yml) with `task deploy:up` — compiles images locally. See root [README.md](../README.md).

## Frontend

Not included in this stack. Build [mytonstorage-org](https://github.com/rudolfkova/mytonstorage-org) and serve via nginx with `NEXT_PUBLIC_API_BASE=https://mytonstorage.org` and API proxy to `BACKEND_PORT`.
