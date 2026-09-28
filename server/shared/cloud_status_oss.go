//go:build !billing

package shared

import "hitkeep/api"

func (c *Context) CloudStatus() *api.CloudStatus {
	return nil
}
