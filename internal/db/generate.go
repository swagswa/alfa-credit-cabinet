// Package db: SQL-запросы в queries/*.sql и сгенерированный по ним sqlc код.
// Вызывает его только internal/repo.
package db

//go:generate go tool sqlc generate -f ../../sqlc.yaml
