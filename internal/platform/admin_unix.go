//go:build linux || darwin

package platform

import "os"

func IsAdmin() bool {
	return os.Geteuid() == 0
}
