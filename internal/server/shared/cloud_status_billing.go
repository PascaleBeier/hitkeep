//go:build billing

package shared

import (
	"strings"

	"hitkeep/internal/api"
)

func (c *Context) CloudStatus() *api.CloudStatus {
	if c == nil || c.Config == nil || !c.Config.CloudHosted {
		return nil
	}

	return &api.CloudStatus{
		Hosted:        true,
		SignupEnabled: c.Config.CloudSignupEnabled,
		Jurisdiction:  strings.TrimSpace(c.Config.CloudJurisdiction),
		Region:        strings.TrimSpace(c.Config.CloudRegion),
		UpgradeURL:    strings.TrimSpace(c.Config.CloudUpgradeURL),
		SupportURL:    strings.TrimSpace(c.Config.CloudSupportURL),
	}
}
