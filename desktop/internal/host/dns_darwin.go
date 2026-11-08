//go:build darwin

package host

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// На macOS DNS-запросы идут к роутеру: он в локальной сети, и запросы к нему проходят
// мимо TUN-адаптера — их видит провайдер. Пока работает «весь трафик», ставим всем
// сетевым службам публичные DNS: запросы к ним идут через адаптер, ядро отправляет их
// через VPN. Прежние настройки запоминаются в файле и возвращаются при выключении
// (и при следующем запуске, если приложение было закрыто аварийно).
var tunDNSServers = []string{"1.1.1.1", "8.8.8.8"}

var dnsMu sync.Mutex

func dnsBackupFile() string { return filepath.Join(dataDir(), "dns-backup.json") }

func netServices() []string {
	out, err := exec.Command("networksetup", "-listallnetworkservices").Output()
	if err != nil {
		return nil
	}
	var res []string
	for i, l := range strings.Split(string(out), "\n") {
		l = strings.TrimSpace(l)
		if i == 0 || l == "" || strings.HasPrefix(l, "*") { // заголовок и выключенные службы
			continue
		}
		res = append(res, l)
	}
	return res
}

func flushDNS() {
	exec.Command("dscacheutil", "-flushcache").Run()
	exec.Command("killall", "-HUP", "mDNSResponder").Run()
}

func tunDNSUp() {
	dnsMu.Lock()
	defer dnsMu.Unlock()
	services := netServices()
	backup := map[string][]string{}
	if b, err := os.ReadFile(dnsBackupFile()); err != nil || json.Unmarshal(b, &backup) != nil {
		backup = map[string][]string{}
		for _, s := range services {
			out, _ := exec.Command("networksetup", "-getdnsservers", s).Output()
			var list []string
			for _, l := range strings.Split(string(out), "\n") {
				l = strings.TrimSpace(l)
				if l != "" && !strings.Contains(l, " ") { // «There aren't any DNS Servers set…» — автоматически
					list = append(list, l)
				}
			}
			backup[s] = list
		}
		if b, err := json.Marshal(backup); err == nil {
			os.WriteFile(dnsBackupFile(), b, 0o600)
		}
	}
	for _, s := range services {
		if err := exec.Command("networksetup", append([]string{"-setdnsservers", s}, tunDNSServers...)...).Run(); err != nil {
			log.Printf("dns: %s: %v", s, err)
		}
	}
	flushDNS()
}

func tunDNSDown() {
	dnsMu.Lock()
	defer dnsMu.Unlock()
	b, err := os.ReadFile(dnsBackupFile())
	if err != nil {
		return
	}
	backup := map[string][]string{}
	if json.Unmarshal(b, &backup) == nil {
		for s, list := range backup {
			args := []string{"-setdnsservers", s}
			if len(list) == 0 {
				args = append(args, "Empty")
			} else {
				args = append(args, list...)
			}
			exec.Command("networksetup", args...).Run()
		}
	}
	os.Remove(dnsBackupFile())
	flushDNS()
}
