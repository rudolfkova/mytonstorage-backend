# mytonstorage-backend

**[Russian version](README.ru.md)**

Backend for [mytonstorage.org](https://mytonstorage.org).

## Description

Backend for uploading and managing files on TON Storage:
- File uploads to TON Storage
- Storage contract lifecycle (init, top-up, withdrawal, provider updates)
- TON Connect authentication
- Monitors storage contracts and notifies providers about new bags
- REST API for the frontend
- Prometheus metrics

## Local development (Docker)

**Requires:** [Docker](https://docs.docker.com/), [Task](https://taskfile.dev), Go 1.25+ (for local builds outside containers).

Container stack: **PostgreSQL**, **tonutils-storage** (v1.5.1), **backend**.

```bash
task deploy:up       # creates deploy/.env on first run
task deploy:health
task deploy:logs
task deploy:down
task deploy:reset    # stop and remove volumes
```

API: `http://localhost:9092` (`BACKEND_PORT` in `deploy/.env`).

Copy `deploy/.env.example` to `deploy/.env`, or run `task deploy:init`.  
`DB_USER` must be **pguser** (see `db/init.sql`).

### Frontend

Dev backend (`BACKEND_BUILD_TAGS=debug`) enables CORS for `http://localhost:3000`.

1. Clone [mytonstorage-org](https://github.com/dearjohndoe/mytonstorage-org)
2. Point `lib/api.ts` to `http://localhost:9092`
3. `npm install && npm run dev`
4. Keep `SYSTEM_HOST=localhost` in `deploy/.env` for TON Connect

## VPS deploy (Docker Hub)

**Recommended for production:** build images on a dev machine; on the VPS only `pull` + compose — no Go compile on the server.

| Image | Task (dev machine) |
|-------|-------------------|
| backend | `BACKEND_IMAGE=<user>/mytonstorage-backend:latest task image:build:backend` |
| tonutils-storage | `TONUTILS_STORAGE_IMAGE=<user>/mytonutils-storage:v1.5.1 task image:build:tonutils-storage` |
| both | `BACKEND_IMAGE=... TONUTILS_STORAGE_IMAGE=... task image:build:push` |

For a release backend image leave `BUILD_TAGS` unset (or empty).

On the VPS (`deploy/docker-compose.hub.yml`, `deploy/.env.hub`, `deploy/secrets/agents-ca.crt`):

```bash
task hub:init
nano deploy/.env.hub
task hub:up
task hub:health
```

Set `BACKEND_IMAGE`, `TONUTILS_STORAGE_IMAGE`, `SYSTEM_HOST=mytonstorage.org`, `AGENT_*`, and `TONUTILS_STORAGE_EXTERNAL_IP` in `.env.hub`.

Frontend is deployed separately — [mytonstorage-org](https://github.com/rudolfkova/mytonstorage-org) static build + nginx.

More detail: [deploy/README.md](deploy/README.md).

## Local build (no Docker)

```bash
task build
task test:build
```

For the full flow use `task deploy:up`.

## Project layout

```
├── cmd/
├── pkg/
├── db/
├── deploy/
├── Dockerfile
└── Taskfile.yml
```

## License

Apache-2.0

Created for the TON Foundation community.
