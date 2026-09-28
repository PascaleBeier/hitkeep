package tenant

import "embed"

//go:embed *.sql
var Fs embed.FS
