package duckdbextensions

import "embed"

//go:embed assets/osx_arm64/*.gz LICENSE.*
var archives embed.FS
