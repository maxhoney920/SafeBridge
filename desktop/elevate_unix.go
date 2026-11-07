//go:build !windows

package main

import "errors"

func relaunchElevated() error {
	return errors.New("запустите SafeBridge с правами администратора (sudo), чтобы включить режим «Весь трафик»")
}
