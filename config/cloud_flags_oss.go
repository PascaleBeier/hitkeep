//go:build !billing

package config

import "flag"

func includeCloudConfigFields() bool {
	return false
}

func registerCloudFlags(_ *flag.FlagSet, _ *Config) {
}
