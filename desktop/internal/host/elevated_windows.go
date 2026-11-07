//go:build windows

package host

import (
	"syscall"
	"unsafe"
)

// IsElevated — запущено ли приложение с правами администратора (нужно для TUN).
func IsElevated() bool {
	var token syscall.Token
	p, _ := syscall.GetCurrentProcess()
	if err := syscall.OpenProcessToken(p, syscall.TOKEN_QUERY, &token); err != nil {
		return false
	}
	defer token.Close()
	var elevation uint32
	var n uint32
	// TokenElevation = 20
	if err := syscall.GetTokenInformation(token, 20, (*byte)(unsafe.Pointer(&elevation)), 4, &n); err != nil {
		return false
	}
	return elevation != 0
}
