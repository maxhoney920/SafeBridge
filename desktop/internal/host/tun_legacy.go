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

// defaultInterface — сетевая карта маршрута по умолчанию (Wi-Fi, Ethernet…).
func defaultInterface() (string, error) {
	var out []byte
	var err error
	switch runtime.GOOS {
	case "darwin":
		out, err = exec.Command("route", "-n", "get", "default").Output()
	default:
		out, err = exec.Command("ip", "route", "show", "default").Output()
	}
	if err != nil {
		return "", errors.New("нет подключения к сети (не найден маршрут по умолчанию)")
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		for i := 0; i+1 < len(f); i++ {
			if f[i] == "interface:" || f[i] == "dev" {
				if f[i+1] != tunName() {
					return f[i+1], nil
				}
			}
		}
	}
	return "", errors.New("нет подключения к сети (не найдена сетевая карта)")
}

func runCmd(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return nil
}

// legacyRoutesUp ждёт адаптер, созданный ядром, и направляет в него весь IPv4-трафик.
func legacyRoutesUp(name string) error {
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
	legacyRoutesDown(name) // остатки прошлого запуска
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
			legacyRoutesDown(name)
			return fmt.Errorf("не удалось направить трафик в адаптер: %v", err)
		}
	}
	return nil
}

func legacyRoutesDown(name string) {
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

// bindOutbound привязывает соединения выхода к сетевой карте (мимо адаптера).
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
