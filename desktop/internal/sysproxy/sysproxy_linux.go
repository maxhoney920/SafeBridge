//go:build linux

package sysproxy

import (
	"os/exec"
	"strings"
)

// В Linux единой настройки нет: поддерживаем GNOME (gsettings) — её же читают
// Chrome и Firefox в большинстве окружений. В остальных окружениях лучше режим TUN.
type linuxBackup struct {
	Mode string `json:"mode"`
	URL  string `json:"url"`
}

func gget(key string) string {
	out, err := exec.Command("gsettings", "get", "org.gnome.system.proxy", key).Output()
	if err != nil {
		return ""
	}
	return strings.Trim(strings.TrimSpace(string(out)), "'")
}

func SetPAC(url string) error {
	if _, err := exec.LookPath("gsettings"); err != nil {
		return err
	}
	saveBackup(linuxBackup{Mode: gget("mode"), URL: gget("autoconfig-url")})
	if err := exec.Command("gsettings", "set", "org.gnome.system.proxy", "autoconfig-url", url).Run(); err != nil {
		return err
	}
	return exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "auto").Run()
}

func Clear() error {
	var b linuxBackup
	if !loadBackup(&b) {
		return nil
	}
	if b.Mode == "" {
		b.Mode = "none"
	}
	exec.Command("gsettings", "set", "org.gnome.system.proxy", "autoconfig-url", b.URL).Run()
	exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", b.Mode).Run()
	dropBackup()
	return nil
}
