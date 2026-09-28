//go:build !billing

package config

func includeCloudConfigFields() bool {
	return false
}
