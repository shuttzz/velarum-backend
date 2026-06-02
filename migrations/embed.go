package migrations

import "embed"

// FS contém as migrations SQL embutidas no binário (usadas pelo goose).
//
//go:embed *.sql
var FS embed.FS
