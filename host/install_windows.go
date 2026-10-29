//go:build windows

package main

import (
	"fmt"
	"os/exec"
)

var windowsRegKeys = []struct{ name, key string }{
	{"Google Chrome", `HKCU\Software\Google\Chrome\NativeMessagingHosts\` + hostName},
	{"Chromium", `HKCU\Software\Chromium\NativeMessagingHosts\` + hostName},
	{"Microsoft Edge", `HKCU\Software\Microsoft\Edge\NativeMessagingHosts\` + hostName},
	{"Brave", `HKCU\Software\BraveSoftware\Brave-Browser\NativeMessagingHosts\` + hostName},
	{"Яндекс Браузер", `HKCU\Software\Yandex\YandexBrowser\NativeMessagingHosts\` + hostName},
}

var firefoxRegKey = `HKCU\Software\Mozilla\NativeMessagingHosts\` + hostName

func registerWindows(manifestPath, firefoxManifest string) ([]string, error) {
	var ok []string
	ff := exec.Command("reg", "add", firefoxRegKey, "/ve", "/t", "REG_SZ", "/d", firefoxManifest, "/f")
	hideWindow(ff)
	if ff.Run() == nil {
		ok = append(ok, "Firefox")
	}
	for _, k := range windowsRegKeys {
		c := exec.Command("reg", "add", k.key, "/ve", "/t", "REG_SZ", "/d", manifestPath, "/f")
		hideWindow(c)
		if out, err := c.CombinedOutput(); err != nil {
			if k.name == "Google Chrome" {
				return nil, fmt.Errorf("reg add: %v: %s", err, out)
			}
			continue
		}
		ok = append(ok, k.name)
	}
	return ok, nil
}

func unregisterWindows() {
	ff := exec.Command("reg", "delete", firefoxRegKey, "/f")
	hideWindow(ff)
	ff.Run()
	for _, k := range windowsRegKeys {
		c := exec.Command("reg", "delete", k.key, "/f")
		hideWindow(c)
		c.Run()
	}
}
