//go:build (!linux && !darwin) || (!amd64 && !arm64)

package duckdbextensions

import "embed"

var archives embed.FS
