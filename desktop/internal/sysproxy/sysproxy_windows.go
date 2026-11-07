//go:build windows

package sysproxy

import (
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const keyPath = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

type winBackup struct {
	AutoConfigURL    string `json:"autoConfigURL"`
	HasAutoConfigURL bool   `json:"hasAutoConfigURL"`
	ProxyEnable      uint32 `json:"proxyEnable"`
}

// SetPAC включает автонастройку прокси по адресу url (для текущего пользователя,
// права администратора не нужны). Браузеры и большинство программ берут её из WinINet.
func SetPAC(url string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	var b winBackup
	if v, _, err := k.GetStringValue("AutoConfigURL"); err == nil {
		b.AutoConfigURL, b.HasAutoConfigURL = v, true
	}
	if v, _, err := k.GetIntegerValue("ProxyEnable"); err == nil {
		b.ProxyEnable = uint32(v)
	}
	saveBackup(b)
	if err := k.SetStringValue("AutoConfigURL", url); err != nil {
		return err
	}
	k.SetDWordValue("ProxyEnable", 0) // ручной прокси выключаем, иначе он важнее PAC
	refresh()
	return nil
}

// Clear возвращает настройки, которые были до приложения.
func Clear() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	var b winBackup
	if loadBackup(&b) {
		if b.HasAutoConfigURL {
			k.SetStringValue("AutoConfigURL", b.AutoConfigURL)
		} else {
			k.DeleteValue("AutoConfigURL")
		}
		k.SetDWordValue("ProxyEnable", b.ProxyEnable)
	} else {
		k.DeleteValue("AutoConfigURL")
	}
	dropBackup()
	refresh()
	return nil
}

var (
	wininet           = syscall.NewLazyDLL("wininet.dll")
	internetSetOption = wininet.NewProc("InternetSetOptionW")
)

// refresh сообщает программам, что настройки прокси изменились.
func refresh() {
	const settingsChanged, refreshOpt = 39, 37
	internetSetOption.Call(0, settingsChanged, 0, 0)
	internetSetOption.Call(0, refreshOpt, 0, 0)
}
