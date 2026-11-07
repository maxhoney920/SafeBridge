// Package notify — системные уведомления (напоминание продлить подписку). Нажатие на
// уведомление открывает ссылку (продление в боте или личном кабинете), если она есть.
package notify

import (
	"errors"
	"net/url"
)

// Show показывает уведомление. link — https-адрес, который откроется по нажатию (или "").
func Show(title, text, link string) error {
	if link != "" {
		u, err := url.Parse(link)
		if err != nil || u.Scheme != "https" {
			return errors.New("недопустимая ссылка")
		}
	}
	return show(title, text, link)
}
