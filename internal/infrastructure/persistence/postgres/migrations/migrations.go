package migrations

import "embed"

// FS embeds the migration files
//
//go:embed *.sql
var FS embed.FS
