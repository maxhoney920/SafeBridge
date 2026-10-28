package main

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func mustParse(t *testing.T, link string) *Server {
	t.Helper()
	s, err := ParseLink(link)
	if err != nil {
		t.Fatalf("ParseLink(%q): %v", link, err)
	}
	return s
}

// jget достаёт значение по пути; числовой шаг — индекс массива.
func jget(v any, path ...any) any {
	cur := v
	for _, p := range path {
		switch k := p.(type) {
		case string:
			m, ok := cur.(map[string]any)
			if !ok {
				return nil
			}
			cur = m[k]
		case int:
			l, ok := cur.([]any)
			if !ok || k >= len(l) {
				return nil
			}
			cur = l[k]
		}
	}
	return cur
}

func TestVLESSReality(t *testing.T) {
	s := mustParse(t, "vless://b831381d-6324-4d53-ad4f-8cda48b30811@1.2.3.4:443?encryption=none&flow=xtls-rprx-vision&security=reality&sni=www.microsoft.com&fp=firefox&pbk=jNXHt1yRo0vDuchQlIP6Z0ZvjT3KtzVI-T4E7RoLJS0&sid=6ba85179e30d4fc2&spx=%2F&type=tcp&headerType=none#%F0%9F%87%A9%F0%9F%87%AA%20Germany")
	if s.Unsupported != "" {
		t.Fatal(s.Unsupported)
	}
	if s.Name != "🇩🇪 Germany" || s.Security != "reality" || s.Transport != "tcp" {
		t.Fatalf("bad meta: %+v", s)
	}
	ob := s.Outbound
	user := jget(ob, "settings", "vnext", 0, "users", 0)
	if jget(user, "id") != "b831381d-6324-4d53-ad4f-8cda48b30811" || jget(user, "flow") != "xtls-rprx-vision" || jget(user, "encryption") != "none" {
		t.Fatalf("bad user: %v", user)
	}
	r := jget(ob, "streamSettings", "realitySettings")
	if jget(r, "publicKey") != "jNXHt1yRo0vDuchQlIP6Z0ZvjT3KtzVI-T4E7RoLJS0" || jget(r, "shortId") != "6ba85179e30d4fc2" ||
		jget(r, "serverName") != "www.microsoft.com" || jget(r, "fingerprint") != "firefox" || jget(r, "spiderX") != "/" {
		t.Fatalf("bad reality: %v", r)
	}
	if jget(ob, "streamSettings", "network") != "raw" {
		t.Fatal("tcp → raw")
	}
}

func TestVLESSXHTTP(t *testing.T) {
	extra := url_(`{"xPaddingBytes":"100-1000","noGRPCHeader":true}`)
	s := mustParse(t, "vless://u@x.example.com:443?type=xhttp&mode=packet-up&path=%2Fxh&host=cdn.example.com&security=tls&sni=x.example.com&alpn=h2&extra="+extra+"#XHTTP")
	if s.Unsupported != "" {
		t.Fatal(s.Unsupported)
	}
	x := jget(s.Outbound, "streamSettings", "xhttpSettings")
	if jget(x, "path") != "/xh" || jget(x, "mode") != "packet-up" || jget(x, "host") != "cdn.example.com" ||
		jget(x, "extra", "xPaddingBytes") != "100-1000" || s.Transport != "xhttp" {
		t.Fatalf("xhttp: %v", x)
	}
	if jget(s.Outbound, "streamSettings", "tlsSettings", "fingerprint") != "chrome" {
		t.Fatal("tls без fp должен получить chrome")
	}
	s = mustParse(t, "vless://u@x.example.com:443?type=xhttp&security=reality&pbk=K&sni=a.com#XR")
	if jget(s.Outbound, "streamSettings", "xhttpSettings", "mode") != "auto" || jget(s.Outbound, "streamSettings", "security") != "reality" {
		t.Fatal("xhttp+reality")
	}
}

func url_(s string) string {
	return strings.NewReplacer(`{`, "%7B", `}`, "%7D", `"`, "%22", `:`, "%3A", `,`, "%2C").Replace(s)
}

func TestVLESSWebSocketAndOthers(t *testing.T) {
	s := mustParse(t, "vless://u@cdn.example.com:443?type=ws&security=tls&path=%2Fws%3Fed%3D2048&host=front.example.com&pcs=abcd#ws")
	ws := jget(s.Outbound, "streamSettings", "wsSettings")
	if jget(ws, "path") != "/ws?ed=2048" || jget(ws, "host") != "front.example.com" {
		t.Fatalf("ws: %v", ws)
	}
	if jget(s.Outbound, "streamSettings", "tlsSettings", "pinnedPeerCertSha256") != "abcd" {
		t.Fatal("pcs")
	}
	s = mustParse(t, "vless://u@h.com:443?type=grpc&serviceName=svc&mode=multi&security=tls#g")
	if jget(s.Outbound, "streamSettings", "grpcSettings", "serviceName") != "svc" || jget(s.Outbound, "streamSettings", "grpcSettings", "multiMode") != true {
		t.Fatal("grpc")
	}
	s = mustParse(t, "vless://u@h.com:443?type=httpupgrade&path=%2Fup&host=x.com&security=tls#hu")
	if jget(s.Outbound, "streamSettings", "httpupgradeSettings", "host") != "x.com" {
		t.Fatal("httpupgrade")
	}
	s = mustParse(t, "vless://u@h.com:443?type=tcp&headerType=http&host=a.com,b.com&path=%2Fp&security=tls#hh")
	if jget(s.Outbound, "streamSettings", "rawSettings", "header", "type") != "http" {
		t.Fatalf("http header: %v", s.Outbound["streamSettings"])
	}
}

func TestVLESSPlainRules(t *testing.T) {
	if s := mustParse(t, "vless://u@1.2.3.4:80?security=none#p"); s.Unsupported == "" {
		t.Fatal("plain VLESS на публичный адрес должен быть помечен")
	}
	if s := mustParse(t, "vless://u@192.168.1.5:80?security=none#p"); s.Unsupported != "" {
		t.Fatal("plain VLESS в локальной сети разрешён")
	}
	if s := mustParse(t, "vless://u@1.2.3.4:80?security=none&encryption=mlkem768x25519plus.native.0rtt.KEY#p"); s.Unsupported != "" ||
		jget(s.Outbound, "settings", "vnext", 0, "users", 0, "encryption") != "mlkem768x25519plus.native.0rtt.KEY" {
		t.Fatal("VLESS Encryption")
	}
	if s := mustParse(t, "vless://u@h.com:443?type=h2&security=tls#h2"); s.Unsupported == "" {
		t.Fatal("h2 удалён из Xray")
	}
}

func TestTrojan(t *testing.T) {
	s := mustParse(t, "trojan://pass%40word@t.example.com:443?sni=t.example.com#Trojan")
	if jget(s.Outbound, "settings", "servers", 0, "password") != "pass@word" || jget(s.Outbound, "streamSettings", "security") != "tls" {
		t.Fatalf("trojan: %v", s.Outbound)
	}
}

func TestVMess(t *testing.T) {
	j := `{"v":"2","ps":"VMess WS","add":"v.example.com","port":"443","id":"11111111-2222-3333-4444-555555555555","aid":"0","scy":"auto","net":"ws","type":"none","host":"v.example.com","path":"/vm","tls":"tls","sni":"v.example.com"}`
	s := mustParse(t, "vmess://"+base64.StdEncoding.EncodeToString([]byte(j)))
	if s.Name != "VMess WS" || jget(s.Outbound, "streamSettings", "wsSettings", "path") != "/vm" ||
		jget(s.Outbound, "settings", "vnext", 0, "users", 0, "id") != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("vmess: %+v", s.Outbound)
	}
	j2 := strings.Replace(j, `"aid":"0"`, `"aid":"64"`, 1)
	if s := mustParse(t, "vmess://"+base64.StdEncoding.EncodeToString([]byte(j2))); s.Unsupported == "" {
		t.Fatal("alterId>0")
	}
}

func TestShadowsocks(t *testing.T) {
	ui := base64.RawURLEncoding.EncodeToString([]byte("chacha20-ietf-poly1305:secret"))
	s := mustParse(t, "ss://"+ui+"@1.1.1.1:8388#SS")
	srv := jget(s.Outbound, "settings", "servers", 0)
	if jget(srv, "method") != "chacha20-poly1305" || jget(srv, "password") != "secret" || s.Port != 8388 {
		t.Fatalf("ss: %v", srv)
	}
	s = mustParse(t, "ss://2022-blake3-aes-128-gcm:abc%3D%3D@2.2.2.2:443#S2")
	if jget(s.Outbound, "settings", "servers", 0, "password") != "abc==" {
		t.Fatal("ss2022")
	}
	old := base64.StdEncoding.EncodeToString([]byte("aes-256-gcm:pw@3.3.3.3:1234"))
	if s = mustParse(t, "ss://"+old+"#Old"); s.Address != "3.3.3.3" || s.Port != 1234 {
		t.Fatalf("ss legacy: %+v", s)
	}
}

func TestHysteria2(t *testing.T) {
	s := mustParse(t, "hy2://pw@hy.example.com:8443?sni=hy.example.com&obfs=salamander&obfs-password=ob#Hy")
	ss := s.Outbound["streamSettings"]
	if jget(ss, "hysteriaSettings", "auth") != "pw" || jget(ss, "finalmask", "udp", 0, "settings", "password") != "ob" ||
		jget(s.Outbound, "settings", "port") != 8443 {
		t.Fatalf("hy2: %v", s.Outbound)
	}
}

func TestStableID(t *testing.T) {
	a := mustParse(t, "vless://u@h.com:443?security=tls&sni=h.com#Name1")
	b := mustParse(t, "vless://u@h.com:443?sni=h.com&security=tls#Другое имя")
	if a.ID != b.ID {
		t.Fatal("id должен зависеть только от параметров подключения")
	}
	if c := mustParse(t, "vless://u@h.com:8443?security=tls&sni=h.com#Name1"); a.ID == c.ID {
		t.Fatal("разные серверы — разные id")
	}
}

func TestParseSubscriptionBase64(t *testing.T) {
	list := strings.Join([]string{
		"vless://u@a.com:443?security=tls#A",
		"vless://u@a.com:443?security=tls#A-dup",
		"garbage line",
		"vless://u@b.com:443?type=xhttp&security=tls#B",
		"trojan://p@c.com:443#C",
		"vless://u@d.com:443?type=quic#D",
		"vless://broken",
	}, "\r\n")
	res, err := ParseSubscription([]byte(base64.StdEncoding.EncodeToString([]byte(list))))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Servers) != 4 || res.Skipped != 1 {
		t.Fatalf("servers=%d skipped=%d", len(res.Servers), res.Skipped)
	}
	if res.Servers[1].Unsupported != "" || res.Servers[3].Unsupported == "" {
		t.Fatal("XHTTP поддерживается, QUIC — нет")
	}
}

func TestParseSubscriptionXrayJSON(t *testing.T) {
	js := `[{"remarks":"🇳🇱 NL","outbounds":[
	  {"tag":"proxy","protocol":"vless","settings":{"vnext":[{"address":"nl.example.com","port":443,"users":[{"id":"u","encryption":"none"}]}]},
	   "streamSettings":{"network":"xhttp","security":"reality","realitySettings":{"publicKey":"k"},"sockopt":{"dialerProxy":"frag"}}},
	  {"tag":"direct","protocol":"freedom"}]},
	 {"remarks":"🇫🇮 FI","outbounds":[{"tag":"proxy","protocol":"trojan","settings":{"servers":[{"address":"fi.example.com","port":443,"password":"p"}]}}]}]`
	res, err := ParseSubscription([]byte(js))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Servers) != 2 || res.Servers[0].Name != "🇳🇱 NL" || res.Servers[0].Security != "reality" ||
		res.Servers[0].Transport != "xhttp" || res.Servers[0].Address != "nl.example.com" || res.Servers[1].Port != 443 {
		t.Fatalf("%+v %+v", res.Servers[0], res.Servers[1])
	}
	if jget(res.Servers[0].Outbound, "streamSettings", "sockopt", "dialerProxy") != nil {
		t.Fatal("dialerProxy должен быть убран")
	}
	if _, err := ParseSubscription([]byte(`{"outbounds":[{"type":"vless","tag":"x"}]}`)); err == nil {
		t.Fatal("sing-box JSON должен давать понятную ошибку")
	}
}

func TestBuildConfig(t *testing.T) {
	a := mustParse(t, "vless://u@a.com:443?security=tls#A")
	b := mustParse(t, "trojan://p@c.com:443#C")
	l := &coreLayout{port: 2080, apiPort: 9090, testPort: map[string]int{a.ID: 3001, b.ID: 3002}, selected: b.ID}
	cfg, err := buildConfig([]ApplyServer{{a.ID, a.Outbound}, {b.ID, b.Outbound}}, l)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	json.Unmarshal(cfg, &m)
	if bal := jget(m, "routing", "balancers", 0); len(jget(bal, "selector").([]any)) != 2 {
		t.Fatalf("balancer: %v", bal)
	}
	if n := len(m["inbounds"].([]any)); n != 4 {
		t.Fatalf("inbounds=%d", n)
	}
	rules := jget(m, "routing", "rules").([]any)
	if jget(rules[len(rules)-1], "balancerTag") != balancerTag {
		t.Fatal("последнее правило — балансировщик")
	}
}
