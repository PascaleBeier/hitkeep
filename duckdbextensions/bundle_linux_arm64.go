package duckdbextensions

import "embed"

//go:embed assets/linux_arm64/*.gz LICENSE.*
var archives embed.FS
