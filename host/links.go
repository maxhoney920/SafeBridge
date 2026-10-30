package main

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

// Server — один сервер из подписки. Outbound — готовый outbound Xray-core (без tag).
type Server struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Protocol    string         `json:"protocol"`
	Address     string         `json:"address"`
	Port        int            `json:"port"`
	Transport   string         `json:"transport,omitempty"`
	Security    string         `json:"security,omitempty"`
	Outbound    map[string]any `json:"outbound,omitempty"`
	Unsupported string         `json:"unsupported,omitempty"`
}

var errSkip = errors.New("skip")

// ParseLink разбирает одну строку подписки (vless://, vmess://, trojan://, ss://, hysteria2://).
func ParseLink(raw string) (*Server, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "#") || strings.HasPrefix(raw, "//") {
		return nil, errSkip
	}
	i := strings.Index(raw, "://")
	if i <= 0 {
		return nil, errSkip
	}
	scheme := strings.ToLower(raw[:i])
	var (
		s   *Server
		err error
	)
	switch scheme {
	case "vless":
		s, err = parseVLESSLike(raw, "vless")
	case "trojan":
		s, err = parseVLESSLike(raw, "trojan")
	case "vmess":
		s, err = parseVMess(raw)
	case "ss":
		s, err = parseSS(raw)
	case "hysteria2", "hy2":
		s, err = parseHy2(raw)
	default:
		return &Server{Name: shortName(raw), Protocol: scheme, Unsupported: "протокол " + scheme + " не поддерживается"}, nil
	}
	if err != nil {
		return nil, err
	}
	finishServer(s, raw)
	return s, nil
}

func finishServer(s *Server, raw string) {
	if s.Name == "" {
		s.Name = net.JoinHostPort(s.Address, strconv.Itoa(s.Port))
	}
	if s.Outbound != nil {
		s.ID = outboundID(s.Outbound)
	} else {
		s.ID = "x" + hashStr(raw)[:10]
	}
}

func shortName(raw string) string {
	if j := strings.LastIndex(raw, "#"); j >= 0 {
		if n, err := url.PathUnescape(raw[j+1:]); err == nil && n != "" {
			return n
		}
	}
	if len(raw) > 40 {
		return raw[:40] + "…"
	}
	return raw
}

func hashStr(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

// outboundID — стабильный идентификатор по параметрам подключения (без имени и тега),
// чтобы выбор пользователя переживал обновление подписки и смену порядка серверов.
func outboundID(ob map[string]any) string {
	cp := map[string]any{}
	for k, v := range ob {
		if k != "tag" {
			cp[k] = v
		}
	}
	b, _ := json.Marshal(cp) // ключи map сериализуются отсортированными
	return "s" + hashStr(string(b))[:10]
}

func parsePort(p string) (int, error) {
	n, err := strconv.Atoi(p)
	if err != nil || n <= 0 || n > 65535 {
		return 0, fmt.Errorf("некорректный порт %q", p)
	}
	return n, nil
}

func fragmentName(u *url.URL) string {
	name := u.Fragment
	if name == "" && u.RawFragment != "" {
		name, _ = url.PathUnescape(u.RawFragment)
	}
	return strings.TrimSpace(name)
}

// q — параметры ссылки с регистронезависимым доступом.
type q map[string]string

func newQ(v url.Values) q {
	out := q{}
	for k, vals := range v {
		if len(vals) > 0 {
			out[strings.ToLower(k)] = vals[0]
		}
	}
	return out
}

func (m q) get(keys ...string) string {
	for _, k := range keys {
		if v, ok := m[strings.ToLower(k)]; ok && v != "" {
			return v
		}
	}
	return ""
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// isPrivateHost — Xray разрешает VLESS без шифрования только на частные адреса.
func isPrivateHost(h string) bool {
	if a, err := netip.ParseAddr(strings.Trim(h, "[]")); err == nil {
		return a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast()
	}
	h = strings.ToLower(h)
	return h == "localhost" || strings.HasSuffix(h, ".local") || strings.HasSuffix(h, ".lan") || !strings.Contains(h, ".")
}

var fingerprints = map[string]bool{
	"chrome": true, "firefox": true, "edge": true, "safari": true, "360": true, "qq": true,
	"ios": true, "android": true, "random": true, "randomized": true, "randomizednoalpn": true,
}

// streamParams — всё, что в ссылке относится к транспорту и шифрованию.
type streamParams struct {
	network     string
	headerType  string
	host        string
	path        string
	serviceName string
	authority   string
	mode        string
	extra       string
	seed        string

	security    string // none | tls | reality
	sni         string
	alpn        string
	fingerprint string
	publicKey   string
	shortID     string
	spiderX     string
	pqv         string // mldsa65Verify
	pcs         string // pinnedPeerCertSha256
	vcn         string // verifyPeerCertByName
	ech         string // echConfigList
}

func streamFromQuery(pq q) streamParams {
	return streamParams{
		network: pq.get("type", "network"), headerType: pq.get("headerType"), host: pq.get("host"),
		path: pq.get("path"), serviceName: pq.get("serviceName", "service_name"), authority: pq.get("authority"),
		mode: pq.get("mode"), extra: pq.get("extra"), seed: pq.get("seed"),
		security: strings.ToLower(pq.get("security")), sni: pq.get("sni", "peer", "serverName"), alpn: pq.get("alpn"),
		fingerprint: pq.get("fp", "fingerprint"), publicKey: pq.get("pbk", "publicKey", "password"),
		shortID: pq.get("sid", "shortId"), spiderX: pq.get("spx", "spiderX"), pqv: pq.get("pqv"),
		pcs: pq.get("pcs", "pinnedPeerCertSha256"), vcn: pq.get("vcn", "verifyPeerCertByName"), ech: pq.get("ech"),
	}
}

// buildStream собирает streamSettings Xray. Возвращает (настройки, имя транспорта для показа, ошибку «не поддерживается»).
func buildStream(p streamParams, host string) (map[string]any, string, error) {
	ss := map[string]any{}
	network := strings.ToLower(strings.TrimSpace(p.network))
	name := network
	switch network {
	case "", "tcp", "raw":
		ss["network"] = "raw"
		name = "tcp"
		if p.headerType == "http" {
			req := map[string]any{"path": []string{nonEmpty(p.path, "/")}}
			if hs := splitList(p.host); len(hs) > 0 {
				req["headers"] = map[string]any{"Host": hs}
			}
			ss["rawSettings"] = map[string]any{"header": map[string]any{"type": "http", "request": req}}
		} else if p.headerType != "" && p.headerType != "none" {
			return nil, name, fmt.Errorf("tcp с headerType=%s не поддерживается", p.headerType)
		}
	case "ws", "websocket":
		ss["network"] = "ws"
		name = "ws"
		ws := map[string]any{"path": nonEmpty(p.path, "/")}
		if p.host != "" {
			ws["host"] = p.host
		}
		ss["wsSettings"] = ws
	case "grpc", "gun":
		ss["network"] = "grpc"
		name = "grpc"
		g := map[string]any{}
		if p.serviceName != "" {
			g["serviceName"] = p.serviceName
		}
		if p.authority != "" {
			g["authority"] = p.authority
		}
		if strings.EqualFold(p.mode, "multi") {
			g["multiMode"] = true
		}
		ss["grpcSettings"] = g
	case "httpupgrade":
		ss["network"] = "httpupgrade"
		h := map[string]any{"path": nonEmpty(p.path, "/")}
		if p.host != "" {
			h["host"] = p.host
		}
		ss["httpupgradeSettings"] = h
	case "xhttp", "splithttp":
		ss["network"] = "xhttp"
		name = "xhttp"
		x := map[string]any{"path": nonEmpty(p.path, "/"), "mode": nonEmpty(strings.ToLower(p.mode), "auto")}
		if p.host != "" {
			x["host"] = p.host
		}
		if p.extra != "" {
			var extra map[string]any
			if err := json.Unmarshal([]byte(p.extra), &extra); err != nil {
				return nil, name, errors.New("некорректный параметр extra у XHTTP")
			}
			x["extra"] = extra
		}
		ss["xhttpSettings"] = x
	case "kcp", "mkcp":
		if (p.headerType != "" && p.headerType != "none") || p.seed != "" {
			return nil, "kcp", errors.New("mKCP с маскировкой или seed не поддерживается")
		}
		ss["network"] = "kcp"
		name = "kcp"
	case "http", "h2":
		return nil, "h2", errors.New("транспорт HTTP/2 удалён из Xray, используйте XHTTP")
	case "quic":
		return nil, "quic", errors.New("транспорт QUIC удалён из Xray")
	default:
		return nil, network, fmt.Errorf("транспорт %s не поддерживается", network)
	}

	switch p.security {
	case "", "none":
		ss["security"] = "none"
	case "tls", "xtls":
		ss["security"] = "tls"
		t := map[string]any{"fingerprint": fp(p.fingerprint)}
		if sni := sniFor(p.sni, host); sni != "" {
			t["serverName"] = sni
		}
		if a := splitList(p.alpn); len(a) > 0 {
			t["alpn"] = a
		}
		if p.pcs != "" {
			t["pinnedPeerCertSha256"] = p.pcs
		}
		if p.vcn != "" {
			t["verifyPeerCertByName"] = p.vcn
		}
		if p.ech != "" {
			t["echConfigList"] = p.ech
		}
		ss["tlsSettings"] = t
	case "reality":
		if p.publicKey == "" {
			return nil, name, errors.New("Reality без pbk")
		}
		ss["security"] = "reality"
		r := map[string]any{"publicKey": p.publicKey, "fingerprint": fp(p.fingerprint)}
		if sni := sniFor(p.sni, host); sni != "" {
			r["serverName"] = sni
		}
		if p.shortID != "" {
			r["shortId"] = p.shortID
		}
		if p.spiderX != "" {
			r["spiderX"] = p.spiderX
		}
		if p.pqv != "" {
			r["mldsa65Verify"] = p.pqv
		}
		ss["realitySettings"] = r
	default:
		return nil, name, fmt.Errorf("security=%s не поддерживается", p.security)
	}
	return ss, name, nil
}

func fp(s string) string {
	s = strings.ToLower(s)
	if fingerprints[s] {
		return s
	}
	return "chrome"
}

func sniFor(sni, host string) string {
	if sni != "" {
		return sni
	}
	if net.ParseIP(strings.Trim(host, "[]")) == nil {
		return host
	}
	return ""
}

func nonEmpty(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func securityName(s string) string {
	switch s {
	case "", "none":
		return "none"
	case "xtls":
		return "tls"
	}
	return s
}

// parseVLESSLike — vless:// и trojan:// имеют одинаковый формат параметров.
func parseVLESSLike(raw, proto string) (*Server, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", proto, err)
	}
	host := u.Hostname()
	port, err := parsePort(u.Port())
	if err != nil || host == "" {
		return nil, fmt.Errorf("%s: нет адреса или порта", proto)
	}
	cred := u.User.Username()
	if pw, ok := u.User.Password(); ok && proto == "trojan" {
		cred = cred + ":" + pw
	}
	if cred == "" {
		return nil, fmt.Errorf("%s: нет UUID/пароля", proto)
	}
	pq := newQ(u.Query())
	sp := streamFromQuery(pq)
	if proto == "trojan" && sp.security == "" {
		sp.security = "tls"
	}
	s := &Server{Name: fragmentName(u), Protocol: proto, Address: host, Port: port, Security: securityName(sp.security)}

	stream, trName, trErr := buildStream(sp, host)
	s.Transport = trName
	if trErr != nil {
		s.Unsupported = trErr.Error()
		return s, nil
	}

	ob := map[string]any{"protocol": proto, "streamSettings": stream}
	if proto == "vless" {
		enc := pq.get("encryption")
		if enc == "" {
			enc = "none"
		}
		if enc == "none" && s.Security == "none" && !isPrivateHost(host) {
			s.Unsupported = "VLESS без TLS/Reality на публичный адрес запрещён в Xray"
			return s, nil
		}
		user := map[string]any{"id": cred, "encryption": enc}
		if flow := pq.get("flow"); flow != "" {
			if flow != "xtls-rprx-vision" && flow != "xtls-rprx-vision-udp443" {
				s.Unsupported = "flow " + flow + " не поддерживается"
				return s, nil
			}
			user["flow"] = flow
		}
		ob["settings"] = map[string]any{"vnext": []any{map[string]any{"address": host, "port": port, "users": []any{user}}}}
	} else {
		ob["settings"] = map[string]any{"servers": []any{map[string]any{"address": host, "port": port, "password": cred}}}
	}
	s.Outbound = ob
	return s, nil
}

func decodeB64(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	s = strings.NewReplacer("\r", "", "\n", "", " ", "").Replace(s)
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			return b, nil
		}
	}
	if m := len(s) % 4; m != 0 {
		return decodeB64(s + strings.Repeat("=", 4-m))
	}
	return nil, errors.New("не base64")
}

func parseVMess(raw string) (*Server, error) {
	body := raw[len("vmess://"):]
	frag := ""
	if j := strings.Index(body, "#"); j >= 0 {
		frag, _ = url.PathUnescape(body[j+1:])
		body = body[:j]
	}
	b, err := decodeB64(body)
	if err != nil {
		return nil, errors.New("vmess: ссылка не в формате base64-JSON")
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, errors.New("vmess: некорректный JSON")
	}
	str := func(k string) string {
		switch v := m[k].(type) {
		case string:
			return strings.TrimSpace(v)
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64)
		}
		return ""
	}
	host := str("add")
	port, err := parsePort(str("port"))
	if err != nil || host == "" {
		return nil, errors.New("vmess: нет адреса или порта")
	}
	id := str("id")
	if id == "" {
		return nil, errors.New("vmess: нет UUID")
	}
	s := &Server{Name: nonEmpty(str("ps"), frag), Protocol: "vmess", Address: host, Port: port, Security: securityName(strings.ToLower(str("tls")))}
	if aid, _ := strconv.Atoi(str("aid")); aid > 0 {
		s.Unsupported = "VMess с alterId больше 0 не поддерживается в Xray"
		return s, nil
	}
	sp := streamParams{network: str("net"), headerType: str("type"), host: str("host"), path: str("path"),
		mode: str("mode"), security: strings.ToLower(str("tls")), sni: str("sni"), alpn: str("alpn"), fingerprint: str("fp")}
	if strings.EqualFold(sp.network, "grpc") {
		sp.serviceName = str("path")
	}
	if strings.EqualFold(sp.network, "kcp") {
		sp.seed = str("path")
	}
	stream, trName, trErr := buildStream(sp, host)
	s.Transport = trName
	if trErr != nil {
		s.Unsupported = trErr.Error()
		return s, nil
	}
	cipher := nonEmpty(str("scy"), "auto")
	s.Outbound = map[string]any{"protocol": "vmess", "streamSettings": stream, "settings": map[string]any{
		"vnext": []any{map[string]any{"address": host, "port": port, "users": []any{map[string]any{"id": id, "security": cipher}}}}}}
	return s, nil
}

var ssMethods = map[string]string{
	"2022-blake3-aes-128-gcm": "2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm": "2022-blake3-aes-256-gcm",
	"2022-blake3-chacha20-poly1305": "2022-blake3-chacha20-poly1305",
	"aes-128-gcm":                   "aes-128-gcm", "aes-256-gcm": "aes-256-gcm",
	"chacha20-ietf-poly1305": "chacha20-poly1305", "chacha20-poly1305": "chacha20-poly1305",
	"xchacha20-ietf-poly1305": "xchacha20-poly1305", "xchacha20-poly1305": "xchacha20-poly1305", "none": "none",
}

func parseSS(raw string) (*Server, error) {
	body := raw[len("ss://"):]
	name := ""
	if j := strings.Index(body, "#"); j >= 0 {
		name, _ = url.PathUnescape(body[j+1:])
		body = body[:j]
	}
	query := ""
	if j := strings.Index(body, "?"); j >= 0 {
		query = body[j+1:]
		body = body[:j]
	}
	body = strings.TrimSuffix(body, "/")
	var userinfo, hostport string
	if at := strings.LastIndex(body, "@"); at >= 0 {
		userinfo, hostport = body[:at], body[at+1:]
		if b, err := decodeB64(userinfo); err == nil && strings.Contains(string(b), ":") {
			userinfo = string(b)
		} else if u, err := url.PathUnescape(userinfo); err == nil {
			userinfo = u
		}
	} else {
		b, err := decodeB64(body)
		if err != nil {
			return nil, errors.New("ss: не удалось разобрать ссылку")
		}
		full := string(b)
		at := strings.LastIndex(full, "@")
		if at < 0 {
			return nil, errors.New("ss: не удалось разобрать ссылку")
		}
		userinfo, hostport = full[:at], full[at+1:]
	}
	c := strings.Index(userinfo, ":")
	if c < 0 {
		return nil, errors.New("ss: нет метода шифрования")
	}
	method, password := strings.ToLower(userinfo[:c]), userinfo[c+1:]
	h, p, err := net.SplitHostPort(hostport)
	if err != nil {
		return nil, errors.New("ss: нет адреса или порта")
	}
	port, err := parsePort(p)
	if err != nil {
		return nil, err
	}
	s := &Server{Name: strings.TrimSpace(name), Protocol: "shadowsocks", Address: h, Port: port, Transport: "tcp", Security: "none"}
	if qv, _ := url.ParseQuery(query); qv.Get("plugin") != "" {
		s.Unsupported = "плагины Shadowsocks не поддерживаются"
		return s, nil
	}
	xm, ok := ssMethods[method]
	if !ok {
		s.Unsupported = "метод " + method + " не поддерживается"
		return s, nil
	}
	s.Outbound = map[string]any{"protocol": "shadowsocks", "settings": map[string]any{
		"servers": []any{map[string]any{"address": h, "port": port, "method": xm, "password": password}}}}
	return s, nil
}

func parseHy2(raw string) (*Server, error) {
	if strings.HasPrefix(strings.ToLower(raw), "hy2://") {
		raw = "hysteria2://" + raw[len("hy2://"):]
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("hysteria2: %v", err)
	}
	host := u.Hostname()
	pstr := nonEmpty(u.Port(), "443")
	pq := newQ(u.Query())
	s := &Server{Name: fragmentName(u), Protocol: "hysteria2", Address: host, Transport: "quic", Security: "tls"}
	if strings.ContainsAny(pstr, ",-") || pq.get("mport") != "" {
		s.Unsupported = "диапазоны портов Hysteria2 не поддерживаются"
		return s, nil
	}
	port, err := parsePort(pstr)
	if err != nil || host == "" {
		return nil, errors.New("hysteria2: нет адреса или порта")
	}
	s.Port = port
	auth := u.User.Username()
	if pw, ok := u.User.Password(); ok {
		auth += ":" + pw
	}
	tls := map[string]any{"fingerprint": fp(pq.get("fp")), "alpn": []string{"h3"}}
	if sni := sniFor(pq.get("sni", "peer"), host); sni != "" {
		tls["serverName"] = sni
	}
	if v := pq.get("pinSHA256", "pcs"); v != "" {
		tls["pinnedPeerCertSha256"] = strings.ReplaceAll(strings.ToLower(v), ":", "")
	}
	stream := map[string]any{"network": "hysteria", "security": "tls", "tlsSettings": tls,
		"hysteriaSettings": map[string]any{"version": 2, "auth": auth}}
	if obfs := pq.get("obfs"); obfs != "" && obfs != "none" {
		if obfs != "salamander" {
			s.Unsupported = "obfs " + obfs + " не поддерживается"
			return s, nil
		}
		stream["finalmask"] = map[string]any{"udp": []any{map[string]any{"type": "salamander",
			"settings": map[string]any{"password": pq.get("obfs-password", "obfs_password")}}}}
	}
	s.Outbound = map[string]any{"protocol": "hysteria", "streamSettings": stream,
		"settings": map[string]any{"version": 2, "address": host, "port": port}}
	return s, nil
}

// serversFromXrayJSON достаёт серверы из JSON-подписки Xray: массив полных конфигов
// или один конфиг (так отвечают панели на User-Agent клиентов, которые понимают JSON).
func serversFromXrayJSON(b []byte) ([]*Server, error) {
	var configs []map[string]any
	if err := json.Unmarshal(b, &configs); err != nil {
		var one map[string]any
		if err2 := json.Unmarshal(b, &one); err2 != nil {
			return nil, err
		}
		configs = []map[string]any{one}
	}
	allowed := map[string]bool{"vless": true, "vmess": true, "trojan": true, "shadowsocks": true, "hysteria": true}
	var out []*Server
	for _, cfg := range configs {
		obs, _ := cfg["outbounds"].([]any)
		remarks, _ := cfg["remarks"].(string)
		for _, x := range obs {
			ob, _ := x.(map[string]any)
			if ob == nil {
				continue
			}
			if _, singbox := ob["type"]; singbox {
				return nil, errors.New("это JSON-подписка sing-box: укажите в настройках User-Agent v2rayN")
			}
			proto, _ := ob["protocol"].(string)
			if !allowed[proto] {
				continue
			}
			tag, _ := ob["tag"].(string)
			ob = sanitizeOutbound(ob)
			name := remarks
			if name == "" || (len(obs) > 1 && tag != "" && tag != "proxy") {
				name = nonEmpty(tag, remarks)
			}
			s := &Server{Name: name, Protocol: proto, Outbound: ob}
			if proto == "hysteria" {
				s.Protocol = "hysteria2"
			}
			s.Address, s.Port = xrayTarget(ob)
			if ss, ok := ob["streamSettings"].(map[string]any); ok {
				s.Transport, _ = ss["network"].(string)
				s.Security, _ = ss["security"].(string)
			}
			if s.Transport == "" || s.Transport == "raw" {
				s.Transport = "tcp"
			}
			s.Security = securityName(s.Security)
			finishServer(s, string(b))
			out = append(out, s)
			break // в каждом конфиге берём первый подходящий outbound
		}
	}
	return out, nil
}

func xrayTarget(ob map[string]any) (string, int) {
	st, _ := ob["settings"].(map[string]any)
	pick := func(m map[string]any) (string, int) {
		a, _ := m["address"].(string)
		p, _ := m["port"].(float64)
		return a, int(p)
	}
	for _, k := range []string{"vnext", "servers"} {
		if l, ok := st[k].([]any); ok && len(l) > 0 {
			if m, ok := l[0].(map[string]any); ok {
				return pick(m)
			}
		}
	}
	if st != nil {
		return pick(st)
	}
	return "", 0
}

// dedupe убирает повторы.
func dedupe(in []*Server) []*Server {
	seen := map[string]bool{}
	var out []*Server
	for _, s := range in {
		if seen[s.ID] {
			continue
		}
		seen[s.ID] = true
		out = append(out, s)
	}
	return out
}
