package duckdbextensions

import "embed"

//go:embed assets/windows_amd64/*.gz LICENSE.*
var archives embed.FS
