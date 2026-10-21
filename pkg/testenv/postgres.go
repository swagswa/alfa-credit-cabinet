// Package testenv поднимает Postgres для интеграционных тестов.
//
// Контейнер один на тестовый бинарник: в нём шаблонная база со всеми
// миграциями, каждый тест получает свою копию через CREATE DATABASE ... TEMPLATE.
// Копия создаётся за доли секунды, тесты не видят данных друг друга и не
// требуют очистки. Контейнер убирает reaper testcontainers после выхода.
package testenv

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io/fs"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"go.uber.org/zap"

	"github.com/swagswa/alfa-credit-cabinet/pkg/postgres"
)

const (
	image    = "postgres:18.6"
	template = "template_app"
	timeout  = 2 * time.Minute
)

var (
	once      sync.Once
	adminURL  string
	onceError error
)

// Postgres отдаёт пул к свежей копии базы с миграциями из migrations.
// Без Docker тест пропускается.
func Postgres(t testing.TB, migrations fs.FS) *pgxpool.Pool {
	t.Helper()

	once.Do(func() { adminURL, onceError = start(migrations) })
	if onceError != nil {
		t.Skipf("postgres container: %v", onceError)
	}

	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()

	name := "test_" + suffix(t)
	exec(t, ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()+" TEMPLATE "+template)
	t.Cleanup(func() {
		// FORCE рвёт соединения, которые тест мог не закрыть
		exec(t, context.WithoutCancel(ctx), "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
	})

	pool, err := pgxpool.New(ctx, withDatabase(t, adminURL, name))
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	// Cleanup идут в обратном порядке: пул закроется раньше DROP DATABASE
	t.Cleanup(pool.Close)
	return pool
}

func start(migrations fs.FS) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	c, err := tcpostgres.Run(ctx, image,
		tcpostgres.WithDatabase(template),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		return "", err
	}
	tplURL, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return "", err
	}

	db := postgres.New(zap.NewNop(), postgres.Config{URL: tplURL}, postgres.Migrate(migrations))
	if err := db.Init(ctx); err != nil {
		return "", err
	}
	// пока к шаблону есть подключения, копировать его нельзя
	db.Pool.Close()

	u, err := url.Parse(tplURL)
	if err != nil {
		return "", err
	}
	u.Path = "/postgres"
	return u.String(), nil
}

func exec(t testing.TB, ctx context.Context, sql string) {
	t.Helper()
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(context.WithoutCancel(ctx))
	if _, err := conn.Exec(ctx, sql); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func withDatabase(t testing.TB, raw, name string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	u.Path = "/" + name
	return u.String()
}

func suffix(t testing.TB) string {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return hex.EncodeToString(b)
}
