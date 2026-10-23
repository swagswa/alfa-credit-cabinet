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
	FNSURL   string        `env:"FNS_URL"         envDefault:"http://localhost:8081"`
	BKIURL   string        `env:"BKI_URL"         envDefault:"http://localhost:8081"`
	BankURL  string        `env:"BANK_URL"        envDefault:"http://localhost:8081"`
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
	Sources

	// по умолчанию сроки демо, на проде 5 рабочих дней и 1 рабочий день
	NeedInfoTTL time.Duration `env:"NEED_INFO_TTL" envDefault:"3m"`
	ReviewSLA   time.Duration `env:"REVIEW_SLA"    envDefault:"5m"`
}

type DocsWorker struct {
	Logger
	Postgres
	Temporal
	Recognition
}

// Recognition нужен docs-worker и make eval-docs. Каталог языков tesseract
// берёт сам из TESSDATA_PREFIX, если он не в стандартном месте.
type Recognition struct {
	AIEnabled     bool          `env:"AI_ENABLED"     envDefault:"false"`
	Tesseract     string        `env:"TESSERACT_PATH" envDefault:"tesseract"`
	OCRTimeout    time.Duration `env:"OCR_TIMEOUT"    envDefault:"30s"`
	OllamaURL     string        `env:"OLLAMA_URL"     envDefault:"http://localhost:11434"`
	OllamaModel   string        `env:"OLLAMA_MODEL"   envDefault:"qwen3:4b-instruct-2507-q4_K_M"`
	OllamaTimeout time.Duration `env:"OLLAMA_TIMEOUT" envDefault:"240s"`
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
