// Package migrations отдаёт SQL-миграции, встроенные в бинарник.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
