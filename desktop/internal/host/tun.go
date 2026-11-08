package host

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

func tunName() string {
	switch runtime.GOOS {
	case "darwin":
		return "utun199" // macOS принимает только utunN
	case "windows":
		return "SafeBridge"
	default:
		return "safebridge0"
	}
}

// hasIPv6 — есть ли у компьютера настоящий IPv6 (не только локальный адрес канала).
// Без него маршрут ::/0 добавить нельзя, и Xray не запустил бы TUN совсем; а утечки
// мимо VPN по IPv6 без IPv6-связи и так нет.
func hasIPv6() bool {
	ifs, err := net.Interfaces()
	if err != nil {
		return false
	}
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() == nil && ipn.IP.IsGlobalUnicast() {
				return true
			}
		}
	}
	return false
}

// На macOS TUN-вход Xray поддерживает только IPv4 (адрес и маршруты).
func tunIPv6() bool { return runtime.GOOS != "darwin" && hasIPv6() }

func tunGateways() []string {
	if tunIPv6() {
		return []string{"172.19.0.1/30", "fdfe:dcba:9876::1/126"}
	}
	return []string{"172.19.0.1/30"}
}

func tunRoutes() []string {
	if tunIPv6() {
		return []string{"0.0.0.0/0", "::/0"}
	}
	return []string{"0.0.0.0/0"}
}

// prepareTun проверяет, что режим «весь трафик» можно включить: нужны права
// администратора, а в Windows ещё и wintun.dll рядом с xray.exe.
func prepareTun(corePath string) error {
	if !IsElevated() {
		return errors.New("для режима «Весь трафик» нужны права администратора")
	}
	if runtime.GOOS == "windows" {
		dll := filepath.Join(filepath.Dir(corePath), "wintun.dll")
		if _, err := os.Stat(dll); err == nil {
			return nil
		}
		if len(embeddedWintun) == 0 {
			return errors.New("в сборке нет wintun.dll")
		}
		if err := os.WriteFile(dll, embeddedWintun, 0o644); err != nil {
			return fmt.Errorf("не удалось положить wintun.dll: %v", err)
		}
	}
	return nil
}

// resolveServerHosts заменяет имена серверов на IP-адреса (для режима TUN).
// Имя остаётся в SNI и заголовке Host, чтобы TLS, Reality и CDN работали как раньше.
func resolveServerHosts(servers []ApplyServer) []ApplyServer {
	out := make([]ApplyServer, 0, len(servers))
	for _, s := range servers {
		ob := deepCopy(s.Outbound).(map[string]any)
		host := ""
		if st, ok := ob["settings"].(map[string]any); ok {
			for _, key := range []string{"vnext", "servers"} {
				if list, ok := st[key].([]any); ok {
					for _, it := range list {
						if m, ok := it.(map[string]any); ok {
							if a, ok := m["address"].(string); ok && net.ParseIP(a) == nil && a != "" {
								if ip := lookup(a); ip != "" {
									host = a
									m["address"] = ip
								}
							}
						}
					}
				}
			}
			if a, ok := st["address"].(string); ok && net.ParseIP(a) == nil && a != "" { // hysteria2, новый формат vless
				if ip := lookup(a); ip != "" {
					host = a
					st["address"] = ip
				}
			}
		}
		if host != "" {
			ss, _ := ob["streamSettings"].(map[string]any)
			if ss == nil {
				ss = map[string]any{}
				ob["streamSettings"] = ss
			}
			if tls, ok := ss["tlsSettings"].(map[string]any); ok && tls["serverName"] == nil {
				tls["serverName"] = host
			} else if ss["security"] == "tls" && ss["tlsSettings"] == nil {
				ss["tlsSettings"] = map[string]any{"serverName": host}
			}
			for _, k := range []string{"wsSettings", "httpupgradeSettings", "xhttpSettings", "splithttpSettings"} {
				if m, ok := ss[k].(map[string]any); ok && (m["host"] == nil || m["host"] == "") {
					m["host"] = host
				}
			}
		}
		out = append(out, ApplyServer{ID: s.ID, Outbound: ob})
	}
	return out
}

func lookup(host string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
	if err != nil || len(ips) == 0 {
		return ""
	}
	return ips[0].String()
}

func deepCopy(v any) any {
	switch t := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, x := range t {
			m[k] = deepCopy(x)
		}
		return m
	case []any:
		l := make([]any, len(t))
		for i, x := range t {
			l[i] = deepCopy(x)
		}
		return l
	default:
		return v
	}
}

// Без IPv6 у компьютера (и на macOS, где TUN только IPv4) ядро не спрашивает AAAA:
// иначе программы получают IPv6-адреса, до которых не дойти.
func dnsQueryStrategy() string {
	if tunIPv6() {
		return "UseIP"
	}
	return "UseIPv4"
}
