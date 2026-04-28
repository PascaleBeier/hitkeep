//go:build !billing

package config

import "flag"

func registerCloudFlags(_ *flag.FlagSet, _ *Config) {
}
