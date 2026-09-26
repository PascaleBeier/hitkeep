package duckdbextensions

import "embed"

//go:embed assets/linux_amd64/*.gz LICENSE.*
var archives embed.FS
