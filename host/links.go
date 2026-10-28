package main

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Server — один сервер из подписки в виде, удобном расширению.
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
	if s.Name == "" {
		s.Name = net.JoinHostPort(s.Address, strconv.Itoa(s.Port))
	}
	if s.Outbound != nil {
		s.ID = outboundID(s.Outbound)
	} else {
		s.ID = "x" + hashStr(raw)[:10]
	}
	return s, nil
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

// outboundID — стабильный идентификатор сервера по содержимому outbound (без тега),
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

func truthy(s string) bool {
	switch strings.ToLower(s) {
	case "1", "true", "yes":
		return true
	}
	return false
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

var utlsFingerprints = map[string]bool{
	"chrome": true, "firefox": true, "edge": true, "safari": true, "360": true, "qq": true,
	"ios": true, "android": true, "random": true, "randomized": true,
}

type tlsParams struct {
	security    string // none | tls | reality
	sni         string
	alpn        string
	fingerprint string
	insecure    bool
	publicKey   string
	shortID     string
}

func buildTLS(p tlsParams, host string) (map[string]any, error) {
	if p.security == "" || p.security == "none" {
		return nil, nil
	}
	if p.security != "tls" && p.security != "reality" && p.security != "xtls" {
		return nil, fmt.Errorf("security=%s не поддерживается", p.security)
	}
	t := map[string]any{"enabled": true}
	sni := p.sni
	if sni == "" && net.ParseIP(host) == nil {
		sni = host
	}
	if sni != "" {
		t["server_name"] = sni
	}
	if p.insecure {
		t["insecure"] = true
	}
	if a := splitList(p.alpn); len(a) > 0 {
		t["alpn"] = a
	}
	fp := strings.ToLower(p.fingerprint)
	if p.security == "reality" {
		if p.publicKey == "" {
			return nil, errors.New("reality без pbk")
		}
		r := map[string]any{"enabled": true, "public_key": p.publicKey}
		if p.shortID != "" {
			r["short_id"] = p.shortID
		}
		t["reality"] = r
		if !utlsFingerprints[fp] {
			fp = "chrome" // для REALITY в sing-box обязателен uTLS
		}
	}
	if utlsFingerprints[fp] {
		t["utls"] = map[string]any{"enabled": true, "fingerprint": fp}
	}
	return t, nil
}

type transportParams struct {
	typ         string
	headerType  string
	host        string
	path        string
	serviceName string
}

// buildTransport переводит транспорт xray-ссылки в транспорт sing-box.
// Возвращает (transport, человекочитаемое имя, ошибку «не поддерживается»).
func buildTransport(p transportParams) (map[string]any, string, error) {
	typ := strings.ToLower(p.typ)
	switch typ {
	case "", "tcp", "raw":
		if p.headerType != "" && p.headerType != "none" {
			return nil, "tcp", fmt.Errorf("tcp с headerType=%s не поддерживается sing-box", p.headerType)
		}
		return nil, "tcp", nil
	case "ws", "websocket":
		t := map[string]any{"type": "ws"}
		path := p.path
		if path == "" {
			path = "/"
		}
		// xray-формат ранних данных: /path?ed=2048
		if u, err := url.Parse(path); err == nil {
			if ed := u.Query().Get("ed"); ed != "" {
				if n, err := strconv.Atoi(ed); err == nil && n > 0 {
					t["max_early_data"] = n
					t["early_data_header_name"] = "Sec-WebSocket-Protocol"
					qv := u.Query()
					qv.Del("ed")
					u.RawQuery = qv.Encode()
					path = u.String()
				}
			}
		}
		t["path"] = path
		if p.host != "" {
			t["headers"] = map[string]any{"Host": p.host}
		}
		return t, "ws", nil
	case "grpc", "gun":
		t := map[string]any{"type": "grpc"}
		if p.serviceName != "" {
			t["service_name"] = p.serviceName
		}
		return t, "grpc", nil
	case "httpupgrade":
		t := map[string]any{"type": "httpupgrade"}
		if p.path != "" {
			t["path"] = p.path
		}
		if p.host != "" {
			t["host"] = p.host
		}
		return t, "httpupgrade", nil
	case "http", "h2":
		t := map[string]any{"type": "http"}
		if p.path != "" {
			t["path"] = p.path
		}
		if hs := splitList(p.host); len(hs) > 0 {
			t["host"] = hs
		}
		return t, "h2", nil
	case "xhttp", "splithttp":
		return nil, typ, errors.New("транспорт XHTTP не поддерживается sing-box")
	default:
		return nil, typ, fmt.Errorf("транспорт %s не поддерживается", typ)
	}
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
	s := &Server{Name: fragmentName(u), Protocol: proto, Address: host, Port: port}

	security := strings.ToLower(pq.get("security"))
	if proto == "trojan" && security == "" {
		security = "tls"
	}
	s.Security = security
	if s.Security == "" {
		s.Security = "none"
	}

	tr, trName, trErr := buildTransport(transportParams{
		typ: pq.get("type"), headerType: pq.get("headerType"), host: pq.get("host"),
		path: pq.get("path"), serviceName: pq.get("serviceName", "service_name"),
	})
	s.Transport = trName
	if trErr != nil {
		s.Unsupported = trErr.Error()
		return s, nil
	}
	tlsObj, err := buildTLS(tlsParams{
		security: security, sni: pq.get("sni", "peer", "serverName"), alpn: pq.get("alpn"),
		fingerprint: pq.get("fp", "fingerprint"), insecure: truthy(pq.get("allowInsecure", "insecure")),
		publicKey: pq.get("pbk", "publicKey"), shortID: pq.get("sid", "shortId"),
	}, host)
	if err != nil {
		s.Unsupported = err.Error()
		return s, nil
	}

	ob := map[string]any{"type": proto, "server": host, "server_port": port}
	if proto == "vless" {
		ob["uuid"] = cred
		if flow := pq.get("flow"); flow != "" {
			if flow != "xtls-rprx-vision" && flow != "xtls-rprx-vision-udp443" {
				s.Unsupported = "flow " + flow + " не поддерживается"
				return s, nil
			}
			ob["flow"] = "xtls-rprx-vision"
		}
		pe := strings.ToLower(pq.get("packetEncoding"))
		if pe == "" || pe == "xudp" {
			ob["packet_encoding"] = "xudp"
		} else if pe == "packetaddr" {
			ob["packet_encoding"] = "packetaddr"
		}
	} else {
		ob["password"] = cred
	}
	if tlsObj != nil {
		ob["tls"] = tlsObj
	}
	if tr != nil {
		ob["transport"] = tr
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
	// иногда паддинг обрезан частично
	if m := len(s) % 4; m != 0 {
		return decodeB64(s + strings.Repeat("=", 4-m))
	}
	return nil, errors.New("не base64")
}

func parseVMess(raw string) (*Server, error) {
	body := strings.TrimPrefix(raw[len("vmess://"):], "")
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
	s := &Server{Name: str("ps"), Protocol: "vmess", Address: host, Port: port}
	if s.Name == "" {
		s.Name = frag
	}
	security := strings.ToLower(str("tls"))
	s.Security = security
	if s.Security == "" {
		s.Security = "none"
	}
	net_ := str("net")
	tp := transportParams{typ: net_, headerType: str("type"), host: str("host"), path: str("path")}
	if strings.EqualFold(net_, "grpc") {
		tp.serviceName = str("path")
	}
	tr, trName, trErr := buildTransport(tp)
	s.Transport = trName
	if trErr != nil {
		s.Unsupported = trErr.Error()
		return s, nil
	}
	tlsObj, err := buildTLS(tlsParams{security: security, sni: str("sni"), alpn: str("alpn"), fingerprint: str("fp"),
		insecure: truthy(str("allowInsecure"))}, host)
	if err != nil {
		s.Unsupported = err.Error()
		return s, nil
	}
	aid, _ := strconv.Atoi(str("aid"))
	cipher := str("scy")
	if cipher == "" {
		cipher = "auto"
	}
	ob := map[string]any{"type": "vmess", "server": host, "server_port": port, "uuid": str("id"),
		"security": cipher, "alter_id": aid, "packet_encoding": "xudp"}
	if ob["uuid"] == "" {
		return nil, errors.New("vmess: нет UUID")
	}
	if tlsObj != nil {
		ob["tls"] = tlsObj
	}
	if tr != nil {
		ob["transport"] = tr
	}
	s.Outbound = ob
	return s, nil
}

var ssMethods = map[string]bool{
	"2022-blake3-aes-128-gcm": true, "2022-blake3-aes-256-gcm": true, "2022-blake3-chacha20-poly1305": true,
	"aes-128-gcm": true, "aes-192-gcm": true, "aes-256-gcm": true, "chacha20-ietf-poly1305": true,
	"xchacha20-ietf-poly1305": true, "none": true,
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
	s := &Server{Name: strings.TrimSpace(name), Protocol: "shadowsocks", Address: h, Port: port, Transport: "tcp"}
	if qv, _ := url.ParseQuery(query); qv.Get("plugin") != "" {
		s.Unsupported = "плагины Shadowsocks не поддерживаются"
		return s, nil
	}
	if !ssMethods[method] {
		s.Unsupported = "метод " + method + " не поддерживается"
		return s, nil
	}
	s.Outbound = map[string]any{"type": "shadowsocks", "server": h, "server_port": port, "method": method, "password": password}
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
	pstr := u.Port()
	if pstr == "" {
		pstr = "443"
	}
	pq := newQ(u.Query())
	s := &Server{Name: fragmentName(u), Protocol: "hysteria2", Address: host, Transport: "quic", Security: "tls"}
	if strings.ContainsAny(pstr, ",-") {
		s.Unsupported = "диапазоны портов hysteria2 не поддерживаются"
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
	tlsObj, _ := buildTLS(tlsParams{security: "tls", sni: pq.get("sni", "peer"), alpn: pq.get("alpn"),
		insecure: truthy(pq.get("insecure", "allowInsecure"))}, host)
	ob := map[string]any{"type": "hysteria2", "server": host, "server_port": port, "password": auth, "tls": tlsObj}
	if obfs := pq.get("obfs"); obfs != "" && obfs != "none" {
		if obfs != "salamander" {
			s.Unsupported = "obfs " + obfs + " не поддерживается"
			return s, nil
		}
		ob["obfs"] = map[string]any{"type": "salamander", "password": pq.get("obfs-password", "obfs_password")}
	}
	s.Outbound = ob
	return s, nil
}

// serversFromSingBoxJSON достаёт поддерживаемые outbounds из готового конфига sing-box
// (так отвечают панели, если подписку запросили с UA sing-box).
func serversFromSingBoxJSON(b []byte) ([]*Server, error) {
	var cfg struct {
		Outbounds []map[string]any `json:"outbounds"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	allowed := map[string]bool{"vless": true, "vmess": true, "trojan": true, "shadowsocks": true, "hysteria2": true}
	var out []*Server
	for _, ob := range cfg.Outbounds {
		typ, _ := ob["type"].(string)
		if !allowed[typ] {
			continue
		}
		name, _ := ob["tag"].(string)
		host, _ := ob["server"].(string)
		port := 0
		if f, ok := ob["server_port"].(float64); ok {
			port = int(f)
		}
		delete(ob, "tag")
		delete(ob, "detour")
		delete(ob, "domain_resolver")
		s := &Server{Name: name, Protocol: typ, Address: host, Port: port, Outbound: ob}
		if tr, ok := ob["transport"].(map[string]any); ok {
			s.Transport, _ = tr["type"].(string)
		} else {
			s.Transport = "tcp"
		}
		if t, ok := ob["tls"].(map[string]any); ok && t["enabled"] == true {
			s.Security = "tls"
			if r, ok := t["reality"].(map[string]any); ok && r["enabled"] == true {
				s.Security = "reality"
			}
		} else {
			s.Security = "none"
		}
		s.ID = outboundID(ob)
		out = append(out, s)
	}
	return out, nil
}

// dedupe убирает повторы и делает теги уникальными.
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
