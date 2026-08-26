//go:build !billing

package entitlements

import "hitkeep/config"

func NewProvider(_ *config.Config) Provider {
	return NewDefaultProvider()
}
