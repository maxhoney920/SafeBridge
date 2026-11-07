//go:build linux

package svc

import (
	"net"

	"golang.org/x/sys/unix"
)

// PeerUID — пользователь процесса на другом конце unix-сокета.
func PeerUID(c net.Conn) (int, bool) {
	uc, ok := c.(*net.UnixConn)
	if !ok {
		return 0, false
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return 0, false
	}
	uid, okk := 0, false
	raw.Control(func(fd uintptr) {
		if cred, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED); err == nil {
			uid, okk = int(cred.Uid), true
		}
	})
	return uid, okk
}
