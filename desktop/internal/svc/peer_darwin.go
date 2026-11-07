//go:build darwin

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
		if cred, err := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED); err == nil {
			uid, okk = int(cred.Uid), true
		}
	})
	return uid, okk
}
