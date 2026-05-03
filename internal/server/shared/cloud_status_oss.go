//go:build !billing

package shared

import "hitkeep/internal/api"

func (c *Context) CloudStatus() *api.CloudStatus {
	return nil
}
