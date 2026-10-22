package testenv

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/redis/go-redis/v9"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

const redisImage = "redis:8.6.0-alpine"

var (
	redisOnce sync.Once
	redisAddr string
	errRedis  error
)

// Redis отдаёт клиента к пустой базе Redis. Контейнер один на тестовый бинарник,
// перед каждым тестом база чистится, поэтому тесты с Redis не запускаем параллельно.
// Без Docker тест пропускается.
func Redis(t testing.TB) *redis.Client {
	t.Helper()

	redisOnce.Do(func() { redisAddr, errRedis = startRedis() })
	if errRedis != nil {
		t.Skipf("redis container: %v", errRedis)
	}

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	t.Cleanup(func() { _ = rdb.Close() })
	if err := rdb.FlushDB(t.Context()).Err(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	return rdb
}

func startRedis() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	c, err := tcredis.Run(ctx, redisImage)
	if err != nil {
		return "", fmt.Errorf("run container: %w", err)
	}
	host, err := c.Host(ctx)
	if err != nil {
		return "", fmt.Errorf("host: %w", err)
	}
	port, err := c.MappedPort(ctx, "6379/tcp")
	if err != nil {
		return "", fmt.Errorf("port: %w", err)
	}
	return host + ":" + port.Port(), nil
}
