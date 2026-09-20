# Деплой mytonstorage-backend на VPS

**[English version](README.md)**

Стек (hub compose):
- `backend` (образ из registry)
- `tonutils-storage` (образ из registry)
- `postgres` (официальный образ)

## Docker Hub (рекомендуется для VPS)

| Шаг | Где | Команда |
|-----|-----|---------|
| 1 | Dev-машина | `BACKEND_IMAGE=... TONUTILS_STORAGE_IMAGE=... task image:build:push` |
| 2 | VPS | Клон репо, `deploy/` + `deploy/secrets/agents-ca.crt` |
| 3 | VPS | `task hub:init` → правка `deploy/.env.hub` |
| 4 | VPS | `task hub:up` |

Hub compose ([docker-compose.hub.yml](docker-compose.hub.yml)) — `pull_policy: always`, без локальной сборки.

### Dev-машина: build + push

```bash
BACKEND_IMAGE=<user>/mytonstorage-backend:latest \
TONUTILS_STORAGE_IMAGE=<user>/mytonutils-storage:v1.5.1 \
task image:build:push
```

Release backend (без debug CORS): не задавай `BUILD_TAGS` или `BUILD_TAGS=`.

### VPS: pull + up

```bash
task hub:init
nano deploy/.env.hub
task hub:up
task hub:health
```

Обязательно в `.env.hub`:
- `BACKEND_IMAGE`, `TONUTILS_STORAGE_IMAGE`
- `SYSTEM_HOST` — домен фронта (TON Connect proof)
- `AGENT_*` — как на coordinator mytonprovider
- `TONUTILS_STORAGE_EXTERNAL_IP` — публичный IP/DNS хоста
- `POSTGRES_PORT` — не конфликтовать с хостовым postgres (например `5433`)

CA агентов: `deploy/secrets/agents-ca.crt` — см. [secrets/README.md](secrets/README.md).

Остановка: `task hub:down`.

## Локальная разработка

[docker-compose.yml](docker-compose.yml) + `task deploy:up` — сборка на месте. См. [README.ru.md](../README.ru.md).

## Фронтенд (отдельный деплой)

UI — репозиторий [mytonstorage-frontend](https://github.com/mytonprovider/mytonstorage-frontend), поднимается **отдельно** от backend.

**Backend + gateway** (этот репо и [mytonstorage-gateway](https://github.com/mytonprovider/mytonstorage-gateway)):

```bash
task hub:up          # mytonstorage-backend — backend на ${BACKEND_PORT:-9092}
task hub:up          # mytonstorage-gateway — gateway на ${GATEWAY_PORT:-9093}
```

**Фронт** (образ с `VITE_API_URL=https://mytonstorage.org`; в `deploy/.env.hub` — `SYSTEM_HOST=mytonstorage.org`):

```bash
task hub:init   # в mytonstorage-frontend
task hub:up     # контейнер на ${PORT:-8082}
```

**Nginx на хосте** для `mytonstorage.org`:

- `location /api/v1/gateway/` → gateway (`127.0.0.1:9093`)
- `location /api/` → backend (`127.0.0.1:9092`)
- `location /` → frontend (`127.0.0.1:8082`)

Каталог провайдеров — с `https://mytonprovider.org` (`VITE_MTPO_URL`), backend stack не меняется.
