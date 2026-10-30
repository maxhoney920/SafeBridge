package main

import "strings"

// Outbound из JSON-подписки приходит с чужого сервера целиком, поэтому из него убирается всё,
// что даёт ядру доступ к файлам и сети компьютера: запись TLS-ключей в файл (masterKeyLog),
// чтение сертификатов с диска, привязка к интерфейсу, цепочки через другие outbound и т.п.
var outboundTopKeys = map[string]bool{"protocol": true, "settings": true, "streamSettings": true, "mux": true}

var forbiddenKeys = map[string]bool{
	"masterkeylog": true, "certificates": true, "certificatefile": true, "keyfile": true,
	"sockopt": true, "dialerproxy": true, "proxysettings": true, "sendthrough": true, "tag": true,
}

func sanitizeOutbound(ob map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range ob {
		if outboundTopKeys[k] {
			out[k] = scrub(v)
		}
	}
	return out
}

func scrub(v any) any {
	switch x := v.(type) {
	case map[string]any:
		m := map[string]any{}
		for k, val := range x {
			if forbiddenKeys[strings.ToLower(k)] {
				continue
			}
			m[k] = scrub(val)
		}
		return m
	case []any:
		l := make([]any, len(x))
		for i, val := range x {
			l[i] = scrub(val)
		}
		return l
	}
	return v
}
