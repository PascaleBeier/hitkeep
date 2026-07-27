package migrations

import "embed"

// Files contains the immutable SQLite control-plane migration stream.
//
//go:embed *.sql
var Files embed.FS
