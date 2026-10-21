# 15. Слои: handler, usecase, repo, domain

Дата: 2026-10-21

## Статус

Принято. Частично заменяет ADR 0010 (раздел про отсутствие слоя repository). Пути и имена пакетов из ADR 0003, 0004, 0009, 0011, 0012, 0013 и 0014 читаются по таблице ниже.

## Контекст

В ADR 0010 и правилах команды домен вызывал сгенерированный `*db.Queries` напрямую, а логика жила в пакетах по предметной области (`credit`, `scoring`, `checks`). Для недели это быстро, но слабо по двум причинам. Типы sqlc становятся доменной моделью: поменяли колонку, поехал домен и ответы API. И бизнес-логику не проверить без Postgres: подмены нет, каждый тест тянет testcontainers.

У части команды есть рабочий опыт с раскладкой handler, usecase, repo, domain на том же стеке (echo, pgx, Temporal, casbin, zap). Жюри смотрит на enterprise-ориентированность, а явные границы слоёв там привычнее всего. Кодогенерацию оставляем: она убирает ручной разбор запросов, а не слои. Инфраструктурный код, которому всё равно, что мы кредитуем (логгер, пул и транзакции Postgres, тестовое окружение), нужен сразу нескольким бинарникам и слоям, и ему нужно своё место отдельно от бизнес-логики.

## Решение

Раскладка бэкенда:

```
cmd/api, cmd/worker, cmd/docs-worker, cmd/mocks, cmd/docgen   сборка зависимостей: config, logger, db -> repo -> usecase -> handler или worker
internal/
  config/          одна структура, caarlos0/env
  domain/          сущности, статусы, роли, таблица переходов, коды причин, sentinel-ошибки
  db/              queries/*.sql и сгенерированный sqlc-код
  repo/            доступ к данным поверх sqlc, по файлу на область
  usecase/         бизнес-логика: заявки, переходы, скоринг, политика, вход, документы
  rules/           загрузка и проверка rules/*.yaml в доменные типы
  rbac/            casbin: model.conf, policy.csv
  integration/     клиенты к внешним системам: egrul, bki, fns, bank, ollama, ocr
  api/oapi/        сгенерированный oapi-codegen (strict server под echo)
  api/handler/     реализация StrictServerInterface
  api/middleware/  JWT из cookie, casbin Enforce
  api/server/      echo, валидатор по спеке, HTTPErrorHandler, SSE
  worker/workflow/ ApplicationWorkflow, RecognizeWorkflow
  worker/activity/ activities
  mocks/           симулятор внешних систем
  docgen/          генератор синтетических документов
pkg/
  logger/          сборка zap, адаптер под логгер Temporal SDK
  postgres/        пул pgx, WithTx, накат миграций golang-migrate
  testenv/         Postgres на testcontainers для интеграционных тестов
```

Зависимости идут в одну сторону: handler и activity -> usecase -> repo -> db. `domain` не импортирует ничего из `internal`, его импортируют все. `pkg` могут импортировать все, сам `pkg` не импортирует ничего из `internal`. Workflow знает только домен и сигнатуры activities.

Правила слоёв:

- **domain.** Чистые типы и функции без I/O: `domain.Application`, `domain.Status`, `domain.CanTransition(from, to, role)`, `domain.AllowedNext(status, role)`, `domain.ErrNotFound`. Тесты обычные юнит.
- **repo.** На область интерфейс и реализация в одном файле: `repo.ApplicationRepository` и неэкспортируемая `applicationRepo`, конструктор возвращает интерфейс. Внутри вызовы `*db.Queries`, наружу только доменные типы: строки sqlc из `repo` не выходят. `pgx.ErrNoRows` превращается в `domain.ErrNotFound` здесь. Метод `WithTx(tx pgx.Tx)` отдаёт тот же репозиторий поверх `db.New(tx)`. Тесты интеграционные на testcontainers.
- **usecase.** Так же: интерфейс `usecase.ApplicationUseCase` и реализация `applicationUseCase`. Здесь решаются транзакции: `postgres.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) error {...})`, внутри репозитории через `WithTx(tx)`. Тесты юнит, репозитории и клиенты подменяются фейками, написанными руками в `_test.go`, без генераторов моков.
- **handler.** Одна структура `handler.Handler` реализует весь `oapi.StrictServerInterface`, методы разложены по файлам областей. Хендлер переводит типы oapi в доменные и обратно и вызывает usecase. Валидацию запроса уже сделал middleware по спеке, пользователя middleware положил в контекст запроса. Типы oapi из `api/` не выходят.
- **pkg.** Переиспользуемый код без знания о предметной области: в нём нет заявок, ИНН, статусов и правил. Пакет называется по тому, что делает (`postgres`, `logger`), а не `utils` или `common`. Код уезжает в `pkg`, когда он инфраструктурный или нужен из двух и более мест; бизнес-логика туда не попадает никогда. Модуль один, `go.mod` общий.
- **worker.** Activities это методы структуры с зависимостями на usecase и integration. Workflow детерминированный, логики в нём минимум.

Смена статуса заявки остаётся в одной точке: `Transition(ctx, appID, to, actor, reason)` у `usecase.ApplicationUseCase`. В одной транзакции: `SELECT ... FOR UPDATE` через репозиторий, проверка `domain.CanTransition`, запись в аудит, `version+1`, `pg_notify`. Вызывается из activity.

Ошибки: sentinel-ошибки в `domain`, слои оборачивают их с полезным контекстом, в HTTP переводит одна функция в `api/server`.

Старые имена из ADR и документов:

| Было | Стало |
|---|---|
| `internal/credit/transitions.go`, `credit.CanTransition`, `AllowedNext` | `internal/domain/transition.go`, `domain.CanTransition`, `domain.AllowedNext` |
| `credit.Transition(ctx, tx, ...)` | `Transition` в `internal/usecase/application.go` |
| `credit.ErrNotFound` и другие ошибки | `domain.ErrNotFound` и т.д. в `internal/domain/errors.go` |
| `credit.ApplicationWorkflow`, `workflow.go`, `activities.go` | `internal/worker/workflow/application.go`, `internal/worker/activity/application.go` |
| `internal/scoring` | `internal/usecase/scoring.go`, `policy.go`; загрузка YAML в `internal/rules` |
| `internal/checks` | `internal/integration/{egrul,bki,fns,bank}` |
| `internal/auth` | `internal/rbac`, `internal/api/middleware`, вход в `internal/usecase/auth.go` |
| `internal/httpapi`, `api.gen.go` | `internal/api/{oapi,handler,server}` |
| `internal/docs`, `docs.RecognizeWorkflow` | `internal/usecase/document.go`, `internal/worker/workflow/recognize.go`, `internal/integration/{ollama,ocr}` |
| домен вызывает `*db.Queries` | домен не знает про БД, `*db.Queries` вызывает только `repo` |

## Последствия

- Схема БД и контракт API меняются, не задевая бизнес-логику: перевод типов живёт на границах, в `repo` и `handler`.
- Скоринг, политику и переходы можно проверить юнит-тестами за миллисекунды, без Postgres.
- Кода больше: на область добавляются интерфейс репозитория, интерфейс usecase и перевод типов. Это сознательная цена. Интерфейсы с одной реализацией тут допустимы, это границы слоёв, а не абстракции на будущее. Внутри слоя новых интерфейсов без второй реализации не заводим.
- Слои не превращаются в пересылку аргументов: если usecase только зовёт репозиторий один в один, это нормально для чтения, но логика (проверки, транзакции, решения) обязана жить в usecase, а не в хендлере и не в SQL.
- Оркестрация, кодогенерация и библиотеки из ADR 0003, 0010-0014 не меняются.
