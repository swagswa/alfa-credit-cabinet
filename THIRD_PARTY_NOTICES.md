# Сторонние компоненты

Всё бесплатное, с открытыми лицензиями. Версии зафиксированы в `go.mod`, `web/package-lock.json`, `Dockerfile` и `compose.yaml`.

## Бэкенд (Go)

| Компонент | Версия | Лицензия |
|---|---|---|
| github.com/labstack/echo/v4 | 4.16.0 | MIT |
| github.com/oapi-codegen/runtime | 1.7.0 | Apache-2.0 |
| github.com/oapi-codegen/echo-middleware | 1.1.0 | Apache-2.0 |
| github.com/getkin/kin-openapi | 0.142.0 | MIT |
| github.com/jackc/pgx/v5 | 5.11.0 | MIT |
| github.com/golang-migrate/migrate/v4 | 4.20.1 | MIT |
| go.temporal.io/sdk | 1.50.0 | MIT |
| go.temporal.io/api | 1.63.5 | MIT |
| github.com/casbin/casbin/v3 | 3.10.0 | Apache-2.0 |
| github.com/golang-jwt/jwt/v5 | 5.3.1 | MIT |
| github.com/redis/go-redis/v9 | 9.23.0 | BSD-2-Clause |
| golang.org/x/crypto (bcrypt) | 0.58.0 | BSD-3-Clause |
| github.com/caarlos0/env/v11 | 11.4.1 | MIT |
| go.uber.org/zap | 1.28.0 | MIT |
| github.com/google/uuid | 1.6.0 | BSD-3-Clause |
| golang.org/x/sync | 0.24.0 | BSD-3-Clause |
| go.yaml.in/yaml/v3 | 3.0.5 | MIT, Apache-2.0 |
| codeberg.org/go-pdf/fpdf | 0.12.0 | MIT |
| golang.org/x/image | 0.47.0 | BSD-3-Clause |
| github.com/ledongthuc/pdf | v0.0.0-20261020125853-5e46805f718e | BSD-3-Clause |
| github.com/pdfcpu/pdfcpu | 0.16.3 | Apache-2.0 |
| github.com/ollama/ollama (пакет api, клиент) | 0.40.7 | MIT |

Только для разработки и тестов, в образы не попадают: oapi-codegen 2.8.0 (Apache-2.0), sqlc 1.31.1 (MIT), workflowcheck 0.6.0 (MIT), testify 1.12.1 (MIT), testcontainers-go и модули postgres и redis 0.44.0 (MIT).

Синтетические документы (`internal/docgen`) набраны шрифтом PT Sans (ParaType, OFL-1.1), файлы шрифта и текст лицензии лежат в `internal/docgen/fonts`.

## Фронт

| Компонент | Версия | Лицензия |
|---|---|---|
| react, react-dom | 19.3.1 | MIT |
| react-router | 7.18.4 | MIT |
| Motion for React (`motion`) | 14.6.0 | MIT |
| @alfalab/core-components | 50.37.0 | MIT |
| openapi-fetch | 0.17.0 | MIT |

Только для сборки и проверок, в образ не попадают: vite 8.3.5 (MIT), @vitejs/plugin-react 6.1.2 (MIT), typescript 5.9.3 (Apache-2.0), openapi-typescript 7.13.0 (MIT), eslint 9.39.5 и @eslint/js (MIT), typescript-eslint 8.72.0 (MIT), eslint-plugin-react-hooks 7.1.1 (MIT), globals 17.13.0 (MIT), @types/react и @types/react-dom 19.3.0 (MIT), @types/node 26.6.5 (MIT), yaml 2.9.1 (ISC, мок API для разработки фронта без бэкенда).

Фирменные шрифты Альфа-Банка не используются, интерфейс на системном шрифте. Логотип банка только в контексте прототипа для Альфа-Хак.

## Образы и сервисы стенда

| Компонент | Версия | Лицензия |
|---|---|---|
| Go (образ сборки golang) | 1.27.3 | BSD-3-Clause |
| Node.js (образ сборки node, alpine) | 24.12.0 | MIT |
| PostgreSQL | 18.6 | PostgreSQL License |
| Redis | 8.6.0 | AGPL-3.0 (из тройной лицензии RSALv2 / SSPLv1 / AGPLv3 выбрана AGPLv3; официальный образ без изменений, отдельный процесс) |
| Temporal server, admin-tools | 1.32.1 | MIT |
| Temporal UI | 2.56.0 | MIT |
| Swagger UI | 5.34.0 | Apache-2.0 |
| nginx (alpine) | 1.29.4 | BSD-2-Clause |
| distroless static (Debian 12) | nonroot | Apache-2.0 |
| Debian | trixie-slim | свободные лицензии пакетов Debian |
| Tesseract OCR, языковые данные rus и eng | из Debian trixie | Apache-2.0 |
| Ollama (профиль ai) | 0.40.7 | MIT |

## Модель (профиль ai)

| Модель | Лицензия |
|---|---|
| Qwen3-4B-Instruct-2507, квантизация q4_K_M (`qwen3:4b-instruct-2507-q4_K_M` в Ollama) | Apache-2.0 |

## Смоделированные интеграции

Реальных обращений к внешним системам нет. За источники отвечает сервис `mocks` (`cmd/mocks`, `internal/mocks`) из фикстур `internal/mocks/fixtures` и генератора `internal/mocks/synth` на синтетических данных, сбои включаются через `/mocks/admin/faults`.

| Система | Как сделано |
|---|---|
| ЕГРЮЛ / ЕГРИП | мок `GET /egrul/{inn}`: реквизиты, статус, руководитель, учредители, ОКВЭД |
| ФНС «Прозрачный бизнес» | мок `GET /fns/{inn}`: выручка, налоговые признаки |
| Бюро кредитных историй | мок `POST /bki/report`, без согласия на запрос 403 |
| Внутренние данные банка | мок `GET /internal/{inn}`: обороты по счёту, продукты, история |
| Примеры документов клиента | мок `GET /api/samples/{inn}`, фронту отдаёт nginx, PDF и PNG рисует тот же генератор, что `testdata/documents` |
| Подписание документов | симуляция кодом в интерфейсе, без СМС и УКЭП; договор PDF из шаблона |
| Выдача кредита | симуляция без движения денег, запись в базе |

## Инструменты разработки

В решение не входят: Playwright 1.64.1 (Apache-2.0) для браузерных проверок, Bruno CLI 4.2.1 (MIT) для прогона коллекции `collections/credit-cabinet`, golangci-lint (GPL-3.0).
