package appurl

import "strings"

// Path joins the configured public URL with an app-owned absolute path.
func Path(publicURL, appPath string) string {
	base := strings.TrimRight(strings.TrimSpace(publicURL), "/")
	path := "/" + strings.TrimLeft(strings.TrimSpace(appPath), "/")
	if base == "" {
		return path
	}
	return base + path
}
