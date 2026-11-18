package migrations

import "embed"

// FS embeds all SQL migration files into the compiled API binary.
//
//go:embed *.sql
var FS embed.FS
