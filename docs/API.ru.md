# mytonstorage-backend — API

Базовый URL (локально): `http://localhost:9092`  
Префикс API: `/api/v1`

## Краткий список ручек

| Метод    | Путь                              | Auth                |
|----------|-----------------------------------|---------------------|
| `GET`    | `/health`                         | —                   |
| `GET`    | `/metrics`                        | Admin Bearer token  |
| `GET`    | `/api/v1/ton-proof`               | —                   |
| `POST`   | `/api/v1/login`                   | —                   |
| `POST`   | `/api/v1/files`                   | Cookie `session_id` |
| `POST`   | `/api/v1/files/unpaid`            | Cookie              |
| `POST`   | `/api/v1/files/paid`              | Cookie              |
| `POST`   | `/api/v1/files/details`           | Cookie              |
| `DELETE` | `/api/v1/files/:bag_id`           | Cookie              |
| `POST`   | `/api/v1/contracts/init-contract` | Cookie              |
| `POST`   | `/api/v1/contracts/topup`         | Cookie              |
| `POST`   | `/api/v1/contracts/withdraw`      | Cookie              |
| `POST`   | `/api/v1/contracts/update`        | Cookie              |
| `POST`   | `/api/v1/providers/offers`        | Cookie              |

---

## Общее

### Авторизация пользователя

1. `GET /api/v1/ton-proof` — получить payload для TON Connect proof (`auth:mytonstorage:<host>`).
2. Кошелёк подписывает proof.
3. `POST /api/v1/login` — бэкенд проверяет proof и выставляет cookie `session_id` (HttpOnly, SameSite=Strict).

Формат cookie: `<hex_signature>:<timestamp>:<user_address>`.

Все защищённые ручки требуют cookie `session_id` (фронт шлёт `withCredentials: true`).

### Авторизация админа

`GET /metrics` — заголовок `Authorization: Bearer <token>`.  
Токен сравнивается с MD5-хешем из конфига (`adminAuthTokens`).

### Rate limit

- Production: 60 запросов / 60 сек (sliding window).
- Debug-сборка: 30 запросов / 60 сек.
- При превышении: `429 Too Many Requests`.

### Ошибки

```json
{ "error": "сообщение" }
```

Успех без тела данных:

```json
{ "status": "ok" }
```

### Лимиты

- Максимальный размер тела запроса: **4 GiB** (загрузка файлов).
- `bag_id` — 64 hex-символа (SHA256 torrent hash).

---

## Служебные

### `GET /health`

Healthcheck. Без авторизации.

**Ответ:** `200` + `{"status":"ok"}`

---

### `GET /metrics`

Prometheus-метрики. Требует admin Bearer token.

**Ответ:** текст Prometheus exposition format.

---

## Авторизация

### `GET /api/v1/ton-proof`

Payload для TON Connect proof. Без авторизации.

**Ответ `200`:**

```json
{ "data": "auth:mytonstorage:localhost" }
```

`host` берётся из `SYSTEM_HOST` в конфиге.

---

### `POST /api/v1/login`

Логин через TON Connect.

**Тело:**

```json
{
  "address": "0:...",
  "state_init": "<base64 или bytes>",
  "proof": {}
}
```

Поле `proof` — объект `wallet.TonConnectProof` из tonutils-go.

**Ответ `200`:** `{"status":"ok"}` + cookie `session_id`.

**Ошибки:**

- `400` — невалидный адрес или proof

---

## Файлы (`/api/v1/files`)

Все ручки группы требуют cookie `session_id`.

### `POST /api/v1/files`

Загрузка файла или папки (multipart).

**Content-Type:** `multipart/form-data`

**Поля:**

- `file` — один или несколько файлов
- `description` — описание bag'а

**Ответ `200`:** `UnpaidBagsResponse`

```json
{
  "bags": [
    {
      "bag_id": "abc...",
      "user_address": "0:...",
      "description": "...",
      "files_count": 1,
      "bag_size": 12345,
      "created_at": 1710000000
    }
  ],
  "free_storage": 86400
}
```

`free_storage` — время бесплатного хранения unpaid bag'ов в секундах.

**Ошибки:**

- `400` — пустое тело, неверный Content-Type, есть неоплаченные bags
- `413` — тело больше 4 GiB
- `503` — не хватает дискового места

**Логика:** файлы сохраняются локально → создаётся bag в tonutils-storage → запись в БД как unpaid.

---

### `POST /api/v1/files/unpaid`

Список неоплаченных bags текущего пользователя.

**Тело:** пустое `{}`

**Ответ `200`:** `UnpaidBagsResponse` (см. выше).

---

### `POST /api/v1/files/paid`

Привязать storage-контракт к bag'у после деплоя/оплаты.

**Тело:**

```json
{
  "bag_id": "abc...",
  "storage_contract": "EQ..."
}
```

**Ответ `200`:** `{"status":"ok"}`

**Ошибки:**

- `400` — невалидный адрес контракта

---

### `POST /api/v1/files/details`

Краткая информация о bags по адресам storage-контрактов.

**Тело:**

```json
{
  "contracts": ["EQ...", "EQ..."]
}
```

**Ответ `200`:** массив `BagInfoShort`

```json
[
  {
    "contract_address": "EQ...",
    "bag_id": "abc...",
    "description": "...",
    "size": 12345
  }
]
```

Лимит: до 1000 контрактов за запрос.

---

### `DELETE /api/v1/files/:bag_id`

Удалить unpaid bag пользователя.

**Параметр:** `bag_id` — 64 hex-символа.

**Ответ `200`:** `{"status":"ok"}`

**Логика:** удаляется связь user↔bag; физическое удаление делает background worker.

**Ошибки:**

- `400` — невалидный `bag_id`

---

## Storage-контракты (`/api/v1/contracts`)

Все ручки требуют cookie `session_id`.

Ручки **не отправляют транзакции в блокчейн** — возвращают данные для подписи в кошельке (`Transaction`).

### `POST /api/v1/contracts/init-contract`

Подготовить деплой storage-контракта v1.

**Тело:**

```json
{
  "providers": ["<provider_pubkey_hex>", "..."],
  "bag_id": "abc...",
  "owner_address": "0:...",
  "amount": 1000000000,
  "span": 86400
}
```

**Логика:**

1. Запрашивает тарифы у провайдеров.
2. Проверяет, что bag не expired.
3. Собирает deploy data (merkle hash, torrent hash, providers).

**Ответ `200`:** `Transaction`

```json
{
  "address": "EQ...",
  "body": "<base64>",
  "state_init": "<base64>",
  "amount": 1000000000
}
```

**Ошибки:**

- `400` — провайдеры недоступны, bag expired, невалидные адреса
- `503` — ошибка подготовки контракта

---

### `POST /api/v1/contracts/topup`

Пополнение баланса storage-контракта.

**Тело:**

```json
{
  "address": "EQ...",
  "amount": 500000000
}
```

**Ответ `200`:** `Transaction` (address + amount; body/state_init могут быть пустыми).

---

### `POST /api/v1/contracts/withdraw`

Закрытие контракта / вывод средств.

**Тело:**

```json
{
  "address": "EQ..."
}
```

**Ответ `200`:** `Transaction`

```json
{
  "body": "<base64 close message>",
  "address": "EQ...",
  "amount": 30000000
}
```

`amount` — ~0.03 TON на gas.

---

### `POST /api/v1/contracts/update`

Смена провайдеров в существующем контракте.

**Тело:**

```json
{
  "providers": ["<pubkey>", "..."],
  "address": "EQ...",
  "bag_size": 12345,
  "amount": 100000000,
  "span": 86400
}
```

**Логика:**

1. Запрашивает тарифы по `bag_size` и `span`.
2. Все провайдеры должны ответить (иначе `400`).
3. Формирует транзакцию обновления контракта.

**Ответ `200`:** `Transaction`

**Таймаут:** 16 сек на запросы к провайдерам.

---

## Провайдеры (`/api/v1/providers`)

### `POST /api/v1/providers/offers`

Получить тарифы провайдеров для bag'а.

**Тело:**

```json
{
  "providers": ["<pubkey>", "..."],
  "bag_id": "abc...",
  "bag_size": 0,
  "span": 86400
}
```

Если `bag_size = 0`, размер берётся из tonutils-storage по `bag_id`.

**Ответ `200`:** `ProviderRatesResponse`

```json
{
  "offers": [
    {
      "offer_span": 86400,
      "price_per_day": 100,
      "price_per_proof": 10,
      "price_per_mb": 5,
      "provider": {
        "key": "...",
        "min_bounty": "...",
        "min_span": 3600,
        "max_span": 604800,
        "price_per_mb_day": 5
      }
    }
  ],
  "declines": [
    { "provider_key": "...", "reason": "..." }
  ]
}
```

**Лимиты:**

- Максимум 50 провайдеров на HTTP-ручке (в сервисе — до 256).
- Таймаут: 16 сек.

---

## Debug-сборка

При `BACKEND_BUILD_TAGS=debug`:

- CORS для `http://localhost:3000` и `http://127.0.0.1:3000`
- Rate limit 30 req/min вместо 60
