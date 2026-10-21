# Кредитный кабинет предпринимателя

Прототип для кейса Альфа-Банка на Альфа-Хак 2026: клиент подбирает кредит и ведёт заявку, банк в своём кабинете проводит её через проверки, скоринг и решение.

Сейчас в репозитории каркас: процессы, инфраструктура, контракт API и схема БД. Бизнес-логики ещё нет, все операции кроме `/api/health` отвечают 501.

## Запуск

Нужен Docker с compose v2. Больше ничего ставить не надо.

```sh
docker compose up -d --build --wait
```

| Что | Адрес |
|---|---|
| Фронт | http://localhost:8000 |
| API | http://localhost:8080/api, проверка: `/api/health` |
| Swagger UI | http://localhost:8082 |
| Temporal UI | http://localhost:8233 |
| Моки внешних систем | http://localhost:8081 |

Распознавание документов (Ollama, Qwen3-4B, Tesseract) в отдельном профиле, при первом запуске скачивается модель около 2,5 ГБ:

```sh
AI_ENABLED=true docker compose --profile ai up -d --build --wait
```

Значения по умолчанию лежат в `compose.yaml`, переопределяются через `.env` (образец в `.env.example`). Если порт занят, поменяй его там же.

## Устройство

Модульный монолит из четырёх процессов: `api`, `worker` (процесс заявки на Temporal), `docs-worker` (распознавание документов) и `mocks`. Слои и пакеты описаны в `CONTRIBUTING.md`, решения в `docs/adr/`, модель данных и статусы в `docs/`.

## Разработка

Go 1.27, Node 24, Docker.

```sh
make generate   # сервер из api/openapi.yaml, код запросов из internal/db/queries
make lint
make test       # интеграционные тесты поднимают Postgres через testcontainers
make check      # всё, что проверяет CI
```

Фронт отдельно: `cd web && npm ci && npm run dev`, dev-сервер проксирует `/api` на `localhost:8080`.
