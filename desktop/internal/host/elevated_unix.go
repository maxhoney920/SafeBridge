//go:build !windows

package host

import "os"

// IsElevated — root (TUN в Linux и macOS требует прав администратора).
func IsElevated() bool { return os.Geteuid() == 0 }
