# 6. docker compose без Kubernetes

Дата: 2026-10-20

## Статус

Принято

## Контекст

Жюри проверяет прототип на обычном ПК командой `docker compose up`, без ручных шагов. Kubernetes для этого требует локального кластера (kind, minikube), лишней памяти и знаний от проверяющего, а команде добавляет манифесты и чарты, которые за неделю никто не проверит в бою.

## Решение

Стенд описан одним `docker-compose.yml`: `db`, `keycloak`, `api`, `worker`, `mocks`, `web`, `swagger`, `riverui`. Ollama с Qwen3 в профиле `ai`, Kafka в профиле `kafka`, базовый стенд работает без них.

- Образы с конкретными тегами, без `latest`.
- Healthcheck у каждого сервиса, `depends_on` по `service_healthy`. Для Go-сервисов healthcheck вызывает тот же бинарь, curl в образе не нужен.
- Секреты в `.env`, в репо `.env.example`.
- Миграции goose и сид при старте `api`, realm Keycloak импортируется из JSON в репо.

## Последствия

- Базовый стенд занимает около 1-1.4 ГБ RAM, с LLM 4-5.5 ГБ.
- Манифестов Kubernetes в репо нет. Путь в k8s описан в README: `api` и `worker` stateless и масштабируются репликами (River раздаёт джобы через `SKIP LOCKED`, миграции под advisory lock), `/healthz` и `/readyz` ложатся в liveness и readiness, конфиг из env переходит в ConfigMap и Secret, Postgres и Keycloak уходят в управляемые сервисы или операторы.
