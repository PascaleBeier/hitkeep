package duckdbextensions

import "embed"

//go:embed assets/osx_amd64/*.gz LICENSE.*
var archives embed.FS
