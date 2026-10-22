# syntax=docker/dockerfile:1

FROM golang:1.27.3 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ ./cmd/...

# api, worker и mocks: один образ, процесс выбирается командой в compose
FROM gcr.io/distroless/static-debian12:nonroot AS app
WORKDIR /app
COPY --from=build /out/api /out/worker /out/mocks /app/
# правила файлами рядом с бинарником, а не вшиты: на стенде их легко подменить томом
COPY rules /app/rules
USER nonroot

# docs-worker отдельно: ему нужен Tesseract, остальным он только раздувает образ
FROM debian:trixie-slim AS docs-worker
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tesseract-ocr tesseract-ocr-rus \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/docs-worker /app/
USER 65532:65532
ENTRYPOINT ["/app/docs-worker"]
