// Package migrations exposes the embedded SQL migration files.
package migrations

import "embed"

//go:embed *.sql
var Files embed.FS
