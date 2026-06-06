# mytonstorage-backend

**[English version](README.md)**

Backend-сервис для mytonstorage.org.

## Описание

Backend для загрузки и управления файлами в TON Storage:
- Загрузка файлов в TON Storage
- Управление жизненным циклом storage-контрактов (инициализация, пополнение, закрытие, обновление провайдеров)
- Авторизация через TON Connect
- Мониторит storage-контракты и уведомляет провайдеров о новых bags для загрузки
- REST API эндпоинты для фронтенд-приложения
- Собирает метрики через **Prometheus**

## Локальная разработка (Docker)

**Нужно:** [Docker](https://docs.docker.com/), [Task](https://taskfile.dev), Go 1.25+ (для локальной сборки вне контейнера).

Стек в контейнерах: **PostgreSQL**, **tonutils-storage** (v1.5.1), **backend**.

```bash
# Поднять стек (создаст deploy/.env при первом запуске)
task deploy:up

# Проверить health
task deploy:health

# Логи
task deploy:logs

# Остановить
task deploy:down

# Остановить и удалить volumes (БД + bags)
task deploy:reset
```

API: `http://localhost:9092` (порт задаётся `BACKEND_PORT` в `deploy/.env`).

Конфиг: скопируй `deploy/.env.example` → `deploy/.env` или запусти `task deploy:init`.  
`DB_USER` должен быть **pguser** — так задано в `db/init.sql`.

### Фронтенд

Backend в dev-сборке (`BACKEND_BUILD_TAGS=debug`) отдаёт CORS для `http://localhost:3000`.

1. Клонируй [mytonstorage-org](https://github.com/dearjohndoe/mytonstorage-org)
2. В `lib/api.ts` укажи `http://localhost:9092` вместо prod URL
3. `npm install && npm run dev`
4. `SYSTEM_HOST=localhost` в `deploy/.env` (для TON Connect proof)

## Локальный запуск без Docker

```bash
task build          # bin/mtpo-backend
task test:build     # проверка compile
```

Для полного flow нужны Postgres и tonutils-storage — проще через `task deploy:up`.

## Структура проекта

```
├── cmd/                   # Точка входа, конфиг, инициализация
├── pkg/                   # Пакеты приложения
│   ├── cache/
│   ├── clients/           # TON blockchain и TON Storage HTTP
│   ├── httpServer/
│   ├── models/
│   ├── repositories/
│   ├── services/
│   └── workers/
├── db/                    # Схема PostgreSQL
├── deploy/                # Docker Compose, .env.example
├── Dockerfile
└── Taskfile.yml
```

## API эндпоинты

- Логин через TON Connect
- Файлы: загрузка, удаление, unpaid bags, краткая инфа
- Контракты: init, topup, withdraw, смена провайдеров
- Провайдеры: offers (тарифы)

## Воркеры

- **Files Worker**: unpaid/expired bags, уведомление провайдеров, проверка загрузки
- **Cleaner Worker**: очистка устаревших данных в БД

## Лицензия

Apache-2.0

Проект создан по заказу участника сообщества TON Foundation.
