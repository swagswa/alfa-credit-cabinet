# Модель данных

Схема в `migrations/` (начало в `000001_init.up.sql`), это первоисточник. Деньги в копейках `bigint`, ставки в базисных пунктах, время `timestamptz` в UTC. Перечисления это `text` с CHECK, значения совпадают с enum в `api/openapi.yaml`. Ключи `uuid` v7 (`uuidv7()` из Postgres 18), чтобы новые строки ложились в конец индекса.

```mermaid
erDiagram
    users ||--o{ company_members : "представляет"
    companies ||--o{ company_members : ""
    companies ||--o{ applications : ""
    users ||--o{ applications : "создал"
    applications ||--o{ application_events : "журнал"
    applications ||--o{ documents : ""
    applications ||--o{ check_results : ""
    applications ||--o{ decisions : ""
    users |o--o{ application_events : "actor"
    users |o--o{ decisions : "decided_by"

    users {
        uuid id PK
        text login UK
        text password_hash
        text full_name
        text role "client manager underwriter admin"
        timestamptz created_at
    }
    companies {
        uuid id PK
        text inn UK
        text kind "UL IP"
        text ogrn
        text kpp "только UL"
        text short_name
        text full_name
        text okved
        text address
        date registered_at
        boolean simulated
        timestamptz created_at
    }
    company_members {
        uuid user_id PK, FK
        uuid company_id PK, FK
        text representative_role "director owner_ip proxy"
        timestamptz created_at
    }
    applications {
        uuid id PK
        bigint number UK
        uuid company_id FK
        uuid created_by FK
        text idempotency_key "UK вместе с created_by"
        bytea request_hash "отпечаток тела создания"
        text status
        integer version
        text product
        text purpose
        bigint amount_kop
        integer term_months
        bigint monthly_revenue_kop
        bigint equipment_cost_kop
        timestamptz bki_consent_at
        timestamptz pd_consent_at
        text comment
        text offer_product
        bigint offer_amount_kop
        integer offer_term_months
        integer offer_rate_bp
        bigint offer_payment_kop
        boolean offer_guarantor
        boolean offer_pledge
        boolean offer_msp_guarantee
        smallint score
        text grade
        text scoring_version
        text policy_version
        timestamptz due_at
        timestamptz submitted_at
        timestamptz created_at
        timestamptz updated_at
    }
    application_events {
        bigint id PK
        uuid application_id FK
        text kind "transition escalation override"
        text from_status
        text to_status
        uuid actor_id FK
        text actor_role "роль или system"
        text reason_codes "text[]"
        text comment
        jsonb payload
        timestamptz created_at
    }
    documents {
        uuid id PK
        uuid application_id FK
        text kind
        text status
        text file_name
        text content_type
        integer size_bytes
        bytea sha256
        bytea content
        uuid uploaded_by FK
        text text_source "pdf_text ocr"
        jsonb extracted_raw
        jsonb extracted
        jsonb validation_errors
        text error
        text model
        text model_digest
        text extractor_version
        integer duration_ms
        jsonb confirmed
        text edited_fields "text[]"
        uuid confirmed_by FK
        timestamptz confirmed_at
        timestamptz created_at
    }
    check_results {
        bigint id PK
        uuid application_id FK
        text source "egrul fns bki internal"
        text status "ok not_found unavailable"
        jsonb payload
        text error
        timestamptz created_at
    }
    decisions {
        bigint id PK
        uuid application_id FK
        text stage "checks policy underwriter"
        text outcome
        text reason_codes "text[]"
        smallint score
        text grade
        jsonb score_breakdown
        jsonb terms
        jsonb recommendation
        text scoring_version
        text policy_version
        uuid decided_by FK
        boolean is_override
        text comment
        timestamptz created_at
    }
```

## Таблицы

**users.** Тестовые учётки из сида, пароль в bcrypt. Роль одна на пользователя, права роли в `internal/rbac/policy.csv`.

**companies.** Компания по ИНН с данными ЕГРЮЛ/ЕГРИП на момент первой заявки. Демо-компании вместе с представителями заранее заводит сид (`migrations/seed/demo.sql`, при `SEED_DEMO=true`). Тип и длина ИНН согласованы в CHECK: 10 цифр у `UL`, 12 у `IP`, КПП только у юрлица. `simulated` ставится компаниям из генератора симулятора, в кабинете у них плашка «данные сгенерированы».

**company_members.** Кто может действовать от имени компании и в какой роли. По этой таблице SQL проверяет владение: клиент видит заявки только своих компаний. Строка появляется при создании первой заявки по ИНН. Полномочия `director` сверяются с руководителем из ЕГРЮЛ, для `proxy` нужна загруженная доверенность.

**applications.** Заявка и её текущее состояние. `status` меняет только `Transition` в `usecase.ApplicationUseCase` под `SELECT ... FOR UPDATE`, каждый раз `version + 1`. `(created_by, idempotency_key)` уникален: повторный POST с тем же ключом отдаёт ту же заявку, а `request_hash` (sha256 тела) отличает настоящий повтор от другого черновика под тем же ключом, тогда 409. С полями сравнивать нельзя: черновик к повтору могли поправить. Поля `offer_*` это действующие условия, которые клиент принимает; история решений в `decisions`. `score` и `grade` продублированы из последнего решения ради фильтров очереди банка. `due_at` это срок текущего шага (ответ клиента, SLA андеррайтера), его же видит клиент.

**application_cards** (view). Заявка с реквизитами компании и ролью автора в ней: из неё читают карточка и список, чтобы набор полей был один.

**application_events.** Журнал аудита, только дописывается: UPDATE и DELETE падают на триггере. `transition` пишется на каждый переход в той же транзакции, `escalation` когда истёк SLA ручного рассмотрения, `override` когда андеррайтер отошёл от рекомендации политики. У `system` нет `actor_id`, у людей есть. Запрос уточнений хранит в `payload` список нужных документов, из него собирается карточка «Нужно от вас».

**documents.** Файл (до 10 МБ, в `bytea`) и всё про распознавание: что вернула модель как есть, что прошло проверку, ошибки проверки, что подтвердил клиент и какие поля он поправил, модель и её digest. Распознаются только `statement` и `financials`, остальные виды всегда `uploaded`. В скоринг идёт только `confirmed`. Смену статуса триггер `documents_notify` шлёт в тот же канал `applications`, что и переходы заявки: статус меняют и api, и docs-worker, а забыть уведомление так нельзя.

**check_results.** Ответы моков ЕГРЮЛ, ФНС, БКИ и внутренних данных по заявке. После `need_info` источники запрашиваются заново новой строкой, актуален последний по каждому источнику, старые остаются для аудита.

**decisions.** История решений: стоп-факторы на `checks`, скоринг и политика на `policy`, ручное решение на `underwriter`. Хранит коды причин, разбивку скоринга по факторам, условия и рекомендацию политики, версии `scoring.yaml` и `policy.yaml`, по которым принято. Решение андеррайтера без комментария не запишется.
