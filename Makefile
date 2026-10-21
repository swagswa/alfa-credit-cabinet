.PHONY: generate lint test check up up-ai down web-gen

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
