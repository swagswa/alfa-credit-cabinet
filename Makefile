.PHONY: generate lint test check up up-ai down web-gen docgen eval-docs

# сервер из openapi, код запросов из sql
generate:
	go generate ./...

lint:
	golangci-lint run ./...
	go tool workflowcheck ./internal/worker/...

# интеграционным тестам нужен Docker, без него они пропускаются
test:
	go test ./...

# то же, что проверяет CI
check: generate lint test
	git diff --exit-code
	cd web && npm run lint && npm run build

up:
	docker compose up -d --build --wait

up-ai:
	AI_ENABLED=true docker compose --profile ai up -d --build --wait

down:
	docker compose --profile ai down

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
