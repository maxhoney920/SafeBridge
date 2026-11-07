//go:build darwin

package sysproxy

import (
	"os/exec"
	"strings"
)

type macBackup struct {
	Services map[string]string `json:"services"` // служба → прежний URL ("" — автонастройка была выключена)
}

func services() []string {
	out, err := exec.Command("networksetup", "-listallnetworkservices").Output()
	if err != nil {
		return nil
	}
	var list []string
	for i, l := range strings.Split(string(out), "\n") {
		l = strings.TrimSpace(l)
		if i == 0 || l == "" || strings.HasPrefix(l, "*") { // первая строка — пояснение, * — выключенные
			continue
		}
		list = append(list, l)
	}
	return list
}

// SetPAC включает «Автонастройку прокси» на всех сетевых службах.
func SetPAC(url string) error {
	b := macBackup{Services: map[string]string{}}
	for _, s := range services() {
		out, _ := exec.Command("networksetup", "-getautoproxyurl", s).Output()
		prev := ""
		if strings.Contains(string(out), "Enabled: Yes") {
			for _, l := range strings.Split(string(out), "\n") {
				if strings.HasPrefix(l, "URL: ") {
					prev = strings.TrimPrefix(l, "URL: ")
				}
			}
		}
		b.Services[s] = prev
	}
	saveBackup(b)
	var last error
	for s := range b.Services {
		if err := exec.Command("networksetup", "-setautoproxyurl", s, url).Run(); err != nil {
			last = err
		}
	}
	return last
}

func Clear() error {
	var b macBackup
	if !loadBackup(&b) {
		return nil
	}
	for s, prev := range b.Services {
		if prev != "" {
			exec.Command("networksetup", "-setautoproxyurl", s, prev).Run()
		} else {
			exec.Command("networksetup", "-setautoproxystate", s, "off").Run()
		}
	}
	dropBackup()
	return nil
}
