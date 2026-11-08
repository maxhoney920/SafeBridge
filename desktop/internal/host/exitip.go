package host

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// ExitIP — внешний адрес, с которым сайты видят пользователя через выбранный сервер.
type ExitIP struct {
	IP      string `json:"ip"`
	Country string `json:"country,omitempty"` // код страны: NL, DE…
	City    string `json:"city,omitempty"`
}

// exitIP спрашивает адрес через локальный SOCKS ядра: вход main всегда ведёт в выбранный
// сервер, при любом режиме списков.
func exitIP(ctx context.Context, port int) (*ExitIP, error) {
	if port <= 0 {
		return nil, errors.New("ядро не запущено")
	}
	p, _ := url.Parse("socks5h://127.0.0.1:" + strconv.Itoa(port))
	var last error
	srcs := []string{"https://ipinfo.io/json", "https://api.ipify.org?format=json"}
	if u := os.Getenv("SAFEBRIDGE_IP_URL"); u != "" { // для проверок
		srcs = []string{u}
	}
	for _, src := range srcs {
		b, err := downloadVia(ctx, src, 64<<10, p)
		if err != nil {
			last = err
			continue
		}
		var r ExitIP
		if json.Unmarshal(b, &r) != nil || net.ParseIP(strings.TrimSpace(r.IP)) == nil {
			last = errors.New("непонятный ответ " + src)
			continue
		}
		r.IP = strings.TrimSpace(r.IP)
		if len(r.Country) != 2 {
			r.Country = ""
		}
		if len(r.City) > 60 {
			r.City = ""
		}
		return &r, nil
	}
	return nil, last
}
