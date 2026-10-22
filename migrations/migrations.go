// Package migrations отдаёт SQL-миграции и демо-сид, встроенные в бинарник.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS

// SeedDemo живёт отдельно от миграций: это данные стенда, а не схема, и на
// окружении без демо он просто не накатывается.
//
//go:embed seed/demo.sql
var SeedDemo string
