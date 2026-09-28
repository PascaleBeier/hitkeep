//go:build !unix

package admin

import "fmt"

func filesystemUsage(path string) (available int64, total int64, err error) {
	return 0, 0, fmt.Errorf("filesystem usage is not supported for %s", path)
}
