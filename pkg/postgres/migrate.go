package postgres

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

// Migrate накатывает миграции из fsys при подключении. Драйвер держит
// advisory lock, так что параллельный старт нескольких api безопасен.
func Migrate(fsys fs.FS) Hook {
	return func(_ context.Context, pool *pgxpool.Pool) error {
		src, err := iofs.New(fsys, ".")
		if err != nil {
			return fmt.Errorf("migrations source: %w", err)
		}
		// Close у такого sql.DB пул не закрывает, только отдаёт соединения
		db := stdlib.OpenDBFromPool(pool)
		driver, err := pgxmigrate.WithInstance(db, &pgxmigrate.Config{})
		if err != nil {
			return fmt.Errorf("migrations driver: %w", err)
		}
		m, err := migrate.NewWithInstance("iofs", src, "pgx5", driver)
		if err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
		// драйвер держит своё соединение из пула до Close, без него Pool.Close повиснет
		defer m.Close()

		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return fmt.Errorf("migrate up: %w", err)
		}
		return nil
	}
}
