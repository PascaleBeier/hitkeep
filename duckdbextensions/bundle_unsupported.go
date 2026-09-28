//go:build (!linux && !darwin && !windows) || (!amd64 && !arm64) || (windows && !amd64)

package duckdbextensions

import "embed"

var archives embed.FS
