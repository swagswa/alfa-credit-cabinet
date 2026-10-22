// Package config читает настройки процессов из переменных окружения.
// Секреты помечены required и без дефолтов: нет секрета, процесс не стартует.
package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"

	"github.com/swagswa/alfa-credit-cabinet/pkg/httpserver"
	"github.com/swagswa/alfa-credit-cabinet/pkg/postgres"
	"github.com/swagswa/alfa-credit-cabinet/pkg/redisdb"
	"github.com/swagswa/alfa-credit-cabinet/pkg/temporalsvc"
)

type Logger struct {
	Level string `env:"LOG_LEVEL" envDefault:"info"`
}

type Postgres struct {
	URL            string        `env:"DATABASE_URL,required"`
	MaxConns       int32         `env:"DATABASE_MAX_CONNS"       envDefault:"10"`
	ConnectTimeout time.Duration `env:"DATABASE_CONNECT_TIMEOUT" envDefault:"5s"`
}

func (p Postgres) Client() postgres.Config {
	return postgres.Config{URL: p.URL, MaxConns: p.MaxConns, ConnectTimeout: p.ConnectTimeout}
}

type Temporal struct {
	HostPort  string `env:"TEMPORAL_HOSTPORT"  envDefault:"localhost:7233"`
	Namespace string `env:"TEMPORAL_NAMESPACE" envDefault:"default"`
}

func (t Temporal) Client() temporalsvc.Config {
	return temporalsvc.Config{HostPort: t.HostPort, Namespace: t.Namespace}
}

type HTTP struct {
	Addr              string        `env:"HTTP_ADDR"                envDefault:":8080"`
	ReadHeaderTimeout time.Duration `env:"HTTP_READ_HEADER_TIMEOUT" envDefault:"5s"`
	ReadTimeout       time.Duration `env:"HTTP_READ_TIMEOUT"        envDefault:"30s"`
	IdleTimeout       time.Duration `env:"HTTP_IDLE_TIMEOUT"        envDefault:"120s"`
}

func (h HTTP) Server() httpserver.Config {
	return httpserver.Config{
		Addr:              h.Addr,
		ReadHeaderTimeout: h.ReadHeaderTimeout,
		ReadTimeout:       h.ReadTimeout,
		IdleTimeout:       h.IdleTimeout,
	}
}

type Redis struct {
	Addr     string `env:"REDIS_ADDR"     envDefault:"localhost:6379"`
	Password string `env:"REDIS_PASSWORD"`
}

func (r Redis) Client() redisdb.Config {
	return redisdb.Config{Addr: r.Addr, Password: r.Password}
}

// Auth настраивает cookie сессии и лимит попыток входа по IP.
type Auth struct {
	// CookieSecure выключаем только для стенда по http, иначе браузер не сохранит cookie
	CookieSecure  bool          `env:"COOKIE_SECURE"  envDefault:"true"`
	LoginAttempts int64         `env:"LOGIN_ATTEMPTS" envDefault:"10"`
	LoginWindow   time.Duration `env:"LOGIN_WINDOW"   envDefault:"1m"`
}

type JWT struct {
	Secret string        `env:"JWT_SECRET,required"`
	TTL    time.Duration `env:"JWT_TTL"             envDefault:"12h"`
}

// Sources это адреса внешних систем, на стенде все они в mocks.
type Sources struct {
	EGRULURL string        `env:"EGRUL_URL"       envDefault:"http://localhost:8081"`
	Timeout  time.Duration `env:"SOURCES_TIMEOUT" envDefault:"3s"`
}

type API struct {
	Logger
	Postgres
	Temporal
	HTTP
	JWT
	Redis
	Auth
	Sources

	// RulesDir относительно рабочей папки: в образе /app/rules, локально rules в корне репо
	RulesDir string `env:"RULES_DIR" envDefault:"rules"`

	// CORSOrigin нужен только dev-серверу Vite, в compose фронт ходит через nginx
	CORSOrigin string `env:"CORS_ORIGIN"`
	AIEnabled  bool   `env:"AI_ENABLED"  envDefault:"false"`
	// SeedDemo заводит тестовые учётки и демо-компании, на стенде для жюри включён
	SeedDemo bool `env:"SEED_DEMO" envDefault:"false"`
}

type Worker struct {
	Logger
	Postgres
	Temporal
}

type DocsWorker struct {
	Logger
	Postgres
	Temporal
}

type Mocks struct {
	Logger
	HTTP
}

func Load[T any]() (T, error) {
	cfg, err := env.ParseAs[T]()
	if err != nil {
		return cfg, fmt.Errorf("config: %w", err)
	}
	return cfg, nil
}
