# 6. docker compose без Kubernetes

Дата: 2026-10-20

## Статус

Принято

## Контекст

Жюри проверяет прототип на обычном ПК командой `docker compose up`, без ручных шагов. Kubernetes для этого требует локального кластера (kind, minikube), лишней памяти и знаний от проверяющего, а команде добавляет манифесты и чарты, которые за неделю никто не проверит в бою.

## Решение

Стенд описан одним `docker-compose.yml`: `db`, `temporal` (плюс одноразовые `temporal-setup` и `temporal-namespace`), `temporal-ui`, `api`, `worker`, `mocks`, `web`, `swagger` (ADR 0012). Ollama с Qwen3 в профиле `ai`, Kafka в профиле `kafka`, базовый стенд работает без них.

- Образы с конкретными тегами, без `latest`.
- Healthcheck у каждого сервиса, `depends_on` по `service_healthy`. Для Go-сервисов healthcheck вызывает тот же бинарь, curl в образе не нужен.
- Секреты в `.env`, в репо `.env.example`.
- Миграции golang-migrate и сид, включая тестовые учётки, при старте `api` (ADR 0010, 0011).

## Последствия

- Базовый стенд занимает около 0.8-1.1 ГБ RAM, с LLM 4-5 ГБ.
- Манифестов Kubernetes в репо нет. Путь в k8s описан в README: `api` и `worker` stateless и масштабируются репликами (задачи воркерам раздаёт task queue Temporal, миграции под advisory lock), `/healthz` и `/readyz` ложатся в liveness и readiness, конфиг из env переходит в ConfigMap и Secret, Postgres уходит в управляемый сервис или оператор, Temporal ставится официальным Helm-чартом или берётся отдельным кластером.
