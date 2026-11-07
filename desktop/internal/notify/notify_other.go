//go:build !linux && !windows && !(darwin && cgo)

package notify

import "errors"

func show(title, text, link string) error {
	return errors.New("уведомления не поддерживаются")
}
