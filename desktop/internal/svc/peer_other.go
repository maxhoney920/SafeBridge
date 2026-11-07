//go:build !linux && !darwin

package svc

import "net"

// PeerUID на других системах не проверяется: доступ к сокету ограничен правами файла.
func PeerUID(c net.Conn) (int, bool) { return 0, false }
