.PHONY: generate lint test check up up-ai up-obs down web-gen docgen eval-docs eval-needs e2e

# сервер из openapi, код запросов из sql
generate:
	go generate ./...

lint:
	golangci-lint run --build-tags e2e ./...
	go tool workflowcheck ./internal/worker/...

# интеграционным тестам нужен Docker, без него они пропускаются
test:
	go test ./...

# то же, что проверяет CI
check: generate lint test
	cd web && npm run gen:api
	git diff --exit-code
	cd web && npm run format:check && npm run lint && npm run build

up:
	docker compose up -d --build --wait

up-ai:
	AI_ENABLED=true docker compose --profile ai up -d --build --wait

# стенд с метриками, логами и трейсами: Grafana :3000, Prometheus :9090, Jaeger :16686
up-obs:
	OTEL_ENDPOINT=http://jaeger:4317 docker compose --profile obs up -d --build --wait

down:
	docker compose --profile ai --profile obs down

web-gen:
	cd web && npm run gen:api

# синтетические выписки и отчётность с эталонами для testdata/documents
docgen:
	go run ./cmd/docgen

# точность распознавания по testdata/documents, пишет docs/ai-eval.md. Нужны tesseract
# и Ollama с моделью (OLLAMA_URL, TESSERACT_PATH как у docs-worker), иначе тест пропускается.
# Один файл: EVAL_GLOB=0000300011_statement_c.png make eval-docs
eval-docs:
	go test -tags eval -run TestDocsEval -count=1 -timeout 60m -v ./internal/usecase

# точность подбора по тексту на testdata/needs.yaml, таблицу из вывода руками в docs/ai-eval.md
eval-needs:
	go test -tags eval -run TestNeedsEval -count=1 -timeout 10m -v ./internal/usecase

# сценарии приёмки через api на поднятом стенде (make up). Лимит входов мешает
# повторным прогонам: LOGIN_ATTEMPTS=100 make up. -short пропускает ожидание expired
e2e:
	go test -tags e2e -count=1 -timeout 10m ./e2e/...
