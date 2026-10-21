package postgres_test

import (
	"context"
	"errors"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/pkg/postgres"
	"github.com/swagswa/alfa-credit-cabinet/pkg/testenv"
)

var schema = fstest.MapFS{
	"000001_t.up.sql":   {Data: []byte("CREATE TABLE t (v int NOT NULL);")},
	"000001_t.down.sql": {Data: []byte("DROP TABLE t;")},
}

func TestWithTx(t *testing.T) {
	pool := testenv.Postgres(t, schema)
	c := &postgres.Client{Pool: pool}
	ctx := t.Context()
	boom := errors.New("boom")

	count := func() int {
		var n int
		require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM t").Scan(&n))
		return n
	}
	insert := func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "INSERT INTO t VALUES (1)")
		return err
	}

	require.NoError(t, c.WithTx(ctx, insert))
	require.Equal(t, 1, count())

	err := c.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		require.NoError(t, insert(ctx, tx))
		return boom
	})
	require.ErrorIs(t, err, boom)
	require.Equal(t, 1, count(), "ошибка откатывает транзакцию")

	calls := 0
	err = c.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		calls++
		if calls < 3 {
			return &pgconn.PgError{Code: "40001"}
		}
		return insert(ctx, tx)
	})
	require.NoError(t, err)
	require.Equal(t, 3, calls, "serialization failure повторяется")
	require.Equal(t, 2, count())
}
