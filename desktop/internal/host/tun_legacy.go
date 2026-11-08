package host

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Сборка для старых macOS (10.13 High Sierra и новее) идёт с Xray 26.2.6: последним,
// который собирается Go 1.25 (новые Xray требуют Go 1.26, а он работает только на
// macOS 12+). Его TUN только создаёт адаптер, поэтому маршруты ставим сами, а соединения
// самого Xray привязываем к сетевой карте — иначе они ушли бы обратно в адаптер.
const legacyCoreVersion = "26.2.6"

func isMacLegacy() bool { return variant == "macos-legacy" }

// legacyRoutes — маршруты «всё через адаптер»: две половины вместо 0.0.0.0/0, чтобы не
// трогать маршрут по умолчанию (он остаётся для соединений к серверу и при выходе).
var legacyRoutes = []string{"0.0.0.0/1", "128.0.0.0/1"}

// defaultRoute — маршрут по умолчанию: сетевая карта (Wi-Fi, Ethernet…) и роутер.
func defaultRoute() (iface, gateway string, err error) {
	var out []byte
	switch runtime.GOOS {
	case "darwin":
		out, err = exec.Command("route", "-n", "get", "default").Output()
	default:
		out, err = exec.Command("ip", "route", "show", "default").Output()
	}
	if err != nil {
		return "", "", errors.New("нет подключения к сети (не найден маршрут по умолчанию)")
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		for i := 0; i+1 < len(f); i++ {
			switch f[i] {
			case "interface:", "dev":
				if f[i+1] != tunName() && iface == "" {
					iface = f[i+1]
				}
			case "gateway:", "via":
				if gateway == "" && net.ParseIP(f[i+1]) != nil {
					gateway = f[i+1]
				}
			}
		}
	}
	if iface == "" {
		return "", "", errors.New("нет подключения к сети (не найдена сетевая карта)")
	}
	return iface, gateway, nil
}

// serverIPs — IPv4-адреса VPN-серверов из готовых выходов (после resolveServerHosts).
func serverIPs(servers []ApplyServer) []string {
	seen := map[string]bool{}
	var out []string
	add := func(a any) {
		if s, ok := a.(string); ok {
			if ip := net.ParseIP(s); ip != nil && ip.To4() != nil && !ip.IsLoopback() && !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	for _, sv := range servers {
		st, _ := sv.Outbound["settings"].(map[string]any)
		if st == nil {
			continue
		}
		for _, key := range []string{"vnext", "servers"} {
			if list, ok := st[key].([]any); ok {
				for _, it := range list {
					if m, ok := it.(map[string]any); ok {
						add(m["address"])
					}
				}
			}
		}
		add(st["address"])
	}
	return out
}

func runCmd(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return nil
}

// legacyRoutesUp ждёт адаптер, созданный ядром, направляет в него весь IPv4-трафик, а
// к самим VPN-серверам — отдельные маршруты через роутер, мимо адаптера (иначе
// соединения ядра с сервером ушли бы по кругу в адаптер).
func legacyRoutesUp(name string, l coreLayout) error {
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := net.InterfaceByName(name); err == nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("адаптер %s не появился", name)
		}
		time.Sleep(100 * time.Millisecond)
	}
	legacyRoutesDown(name, l) // остатки прошлого запуска
	for _, ip := range l.serverIPs {
		var err error
		switch {
		case runtime.GOOS == "darwin" && l.gateway != "":
			err = runCmd("route", "-q", "-n", "add", "-host", ip, l.gateway)
		case runtime.GOOS == "darwin":
			err = runCmd("route", "-q", "-n", "add", "-host", ip, "-interface", l.iface)
		case l.gateway != "":
			err = runCmd("ip", "route", "replace", ip+"/32", "via", l.gateway, "dev", l.iface)
		default:
			err = runCmd("ip", "route", "replace", ip+"/32", "dev", l.iface)
		}
		if err != nil {
			legacyRoutesDown(name, l)
			return fmt.Errorf("не удалось проложить маршрут к серверу %s: %v", ip, err)
		}
	}
	if runtime.GOOS != "darwin" {
		if err := runCmd("ip", "link", "set", name, "up"); err != nil {
			return err
		}
	}
	for _, r := range legacyRoutes {
		var err error
		if runtime.GOOS == "darwin" {
			err = runCmd("route", "-q", "-n", "add", "-net", r, "-interface", name)
		} else {
			err = runCmd("ip", "route", "add", r, "dev", name)
		}
		if err != nil {
			legacyRoutesDown(name, l)
			return fmt.Errorf("не удалось направить трафик в адаптер: %v", err)
		}
	}
	return nil
}

func legacyRoutesDown(name string, l coreLayout) {
	for _, ip := range l.serverIPs {
		if runtime.GOOS == "darwin" {
			exec.Command("route", "-q", "-n", "delete", "-host", ip).Run()
		} else {
			exec.Command("ip", "route", "del", ip+"/32").Run()
		}
	}
	for _, r := range legacyRoutes {
		if runtime.GOOS == "darwin" {
			exec.Command("route", "-q", "-n", "delete", "-net", r, "-interface", name).Run()
		} else {
			exec.Command("ip", "route", "del", r, "dev", name).Run()
		}
	}
}

// legacyTunInbound — TUN-вход в формате Xray 26.2.6 (без автонастройки маршрутов).
func legacyTunInbound() map[string]any {
	return map[string]any{"tag": "tun", "protocol": "tun",
		"settings": map[string]any{"name": tunName(), "MTU": 1500},
		"sniffing": map[string]any{"enabled": true, "destOverride": []string{"http", "tls", "quic"}, "routeOnly": true},
	}
}

// bindOutbound привязывает соединения выхода к сетевой карте (мимо адаптера). Нужна
// только выходу «напрямую»: к VPN-серверам ведут отдельные маршруты через роутер.
func bindOutbound(ob map[string]any, iface string) {
	ss, _ := ob["streamSettings"].(map[string]any)
	if ss == nil {
		ss = map[string]any{}
		ob["streamSettings"] = ss
	}
	so, _ := ss["sockopt"].(map[string]any)
	if so == nil {
		so = map[string]any{}
		ss["sockopt"] = so
	}
	so["interface"] = iface
}
