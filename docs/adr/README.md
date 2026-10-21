# Architecture Decision Records

Формат и правила в [0001](0001-adr.md).

| № | Решение | Статус |
|---|---|---|
| [0001](0001-adr.md) | Ведём ADR | Принято |
| [0002](0002-go-backend.md) | Бэкенд на Go | Принято |
| [0003](0003-openapi-first.md) | Контракт API первым | Принято |
| [0004](0004-state-machine.md) | Машина состояний заявки: своя таблица переходов и River | Заменено [0012](0012-temporal.md) |
| [0005](0005-postgres-queue.md) | Очередь шагов в Postgres (River), Kafka вне ядра | Заменено [0012](0012-temporal.md) |
| [0006](0006-no-kubernetes.md) | docker compose без Kubernetes | Принято |
| [0007](0007-ui-core-components.md) | Интерфейс на @alfalab/core-components | Принято |
| [0008](0008-licenses.md) | Политика лицензий | Принято |
| [0009](0009-scoring-rules.md) | Балльный скоринг на правилах в YAML | Принято |
| [0010](0010-db-access-pgx.md) | Доступ к БД: pgx + sqlc, миграции golang-migrate | Принято |
| [0011](0011-auth-casbin.md) | Свой вход на JWT и роли через casbin вместо Keycloak | Принято |
| [0012](0012-temporal.md) | Процесс заявки на Temporal | Принято |
| [0013](0013-echo.md) | HTTP на echo | Принято |
| [0014](0014-ai-usage.md) | LLM только для распознавания документов, решение без LLM | Принято |
