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

func jget(m map[string]any, path ...string) any {
	var cur any = m
	for _, p := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[p]
	}
	return cur
}

func TestVLESSReality(t *testing.T) {
	s := mustParse(t, "vless://b831381d-6324-4d53-ad4f-8cda48b30811@1.2.3.4:443?encryption=none&flow=xtls-rprx-vision&security=reality&sni=www.microsoft.com&fp=chrome&pbk=jNXHt1yRo0vDuchQlIP6Z0ZvjT3KtzVI-T4E7RoLJS0&sid=6ba85179e30d4fc2&type=tcp&headerType=none#%F0%9F%87%A9%F0%9F%87%AA%20Germany")
	if s.Unsupported != "" {
		t.Fatal(s.Unsupported)
	}
	if s.Name != "🇩🇪 Germany" || s.Security != "reality" || s.Transport != "tcp" {
		t.Fatalf("bad meta: %+v", s)
	}
	ob := s.Outbound
	if ob["uuid"] != "b831381d-6324-4d53-ad4f-8cda48b30811" || ob["flow"] != "xtls-rprx-vision" {
		t.Fatalf("bad ob: %v", ob)
	}
	if jget(ob, "tls", "reality", "public_key") != "jNXHt1yRo0vDuchQlIP6Z0ZvjT3KtzVI-T4E7RoLJS0" ||
		jget(ob, "tls", "reality", "short_id") != "6ba85179e30d4fc2" ||
		jget(ob, "tls", "server_name") != "www.microsoft.com" ||
		jget(ob, "tls", "utls", "fingerprint") != "chrome" {
		t.Fatalf("bad tls: %v", ob["tls"])
	}
	if _, ok := ob["transport"]; ok {
		t.Fatal("tcp must not have transport")
	}
}

func TestVLESSRealityDefaultFingerprint(t *testing.T) {
	s := mustParse(t, "vless://u@h.example:443?security=reality&pbk=KEY&sni=a.com")
	if jget(s.Outbound, "tls", "utls", "fingerprint") != "chrome" {
		t.Fatal("reality без fp должен получить uTLS chrome")
	}
}

func TestVLESSWebSocketEarlyData(t *testing.T) {
	s := mustParse(t, "vless://u@cdn.example.com:443?type=ws&security=tls&path=%2Fws%3Fed%3D2048&host=front.example.com&sni=front.example.com&alpn=h2,http/1.1#ws")
	tr := s.Outbound["transport"].(map[string]any)
	if tr["type"] != "ws" || tr["path"] != "/ws" || tr["max_early_data"] != 2048 ||
		tr["early_data_header_name"] != "Sec-WebSocket-Protocol" {
		t.Fatalf("bad ws: %v", tr)
	}
	if jget(s.Outbound, "transport", "headers", "Host") != "front.example.com" {
		t.Fatal("no host header")
	}
	alpn := jget(s.Outbound, "tls", "alpn").([]string)
	if len(alpn) != 2 {
		t.Fatalf("alpn: %v", alpn)
	}
}

func TestVLESSGrpcAndHTTPUpgrade(t *testing.T) {
	s := mustParse(t, "vless://u@h.com:443?type=grpc&serviceName=svc&security=tls#g")
	if jget(s.Outbound, "transport", "service_name") != "svc" {
		t.Fatal("grpc service_name")
	}
	s = mustParse(t, "vless://u@h.com:80?type=httpupgrade&path=%2Fup&host=x.com#hu")
	if jget(s.Outbound, "transport", "type") != "httpupgrade" || jget(s.Outbound, "transport", "host") != "x.com" {
		t.Fatalf("httpupgrade: %v", s.Outbound["transport"])
	}
	if s.Outbound["tls"] != nil {
		t.Fatal("no tls expected")
	}
}

func TestVLESSXHTTPUnsupported(t *testing.T) {
	s := mustParse(t, "vless://u@h.com:443?type=xhttp&security=tls#x")
	if s.Unsupported == "" || s.Outbound != nil {
		t.Fatal("xhttp must be unsupported")
	}
}

func TestTrojan(t *testing.T) {
	s := mustParse(t, "trojan://pass%40word@t.example.com:443?sni=t.example.com#Trojan")
	if s.Outbound["password"] != "pass@word" || jget(s.Outbound, "tls", "enabled") != true {
		t.Fatalf("trojan: %v", s.Outbound)
	}
}

func TestVMess(t *testing.T) {
	j := `{"v":"2","ps":"VMess WS","add":"v.example.com","port":"443","id":"11111111-2222-3333-4444-555555555555","aid":"0","scy":"auto","net":"ws","type":"none","host":"v.example.com","path":"/vm","tls":"tls","sni":"v.example.com"}`
	s := mustParse(t, "vmess://"+base64.StdEncoding.EncodeToString([]byte(j)))
	if s.Name != "VMess WS" || jget(s.Outbound, "transport", "path") != "/vm" || jget(s.Outbound, "tls", "server_name") != "v.example.com" {
		t.Fatalf("vmess: %+v", s.Outbound)
	}
}

func TestShadowsocks(t *testing.T) {
	ui := base64.RawURLEncoding.EncodeToString([]byte("chacha20-ietf-poly1305:secret"))
	s := mustParse(t, "ss://"+ui+"@1.1.1.1:8388#SS")
	if s.Outbound["method"] != "chacha20-ietf-poly1305" || s.Outbound["password"] != "secret" || s.Port != 8388 {
		t.Fatalf("ss: %v", s.Outbound)
	}
	// SIP002 без base64 (2022-методы) и старый формат «всё в base64»
	s = mustParse(t, "ss://2022-blake3-aes-128-gcm:abc%3D%3D@2.2.2.2:443#S2")
	if s.Outbound["password"] != "abc==" {
		t.Fatalf("ss2022: %v", s.Outbound)
	}
	old := base64.StdEncoding.EncodeToString([]byte("aes-256-gcm:pw@3.3.3.3:1234"))
	s = mustParse(t, "ss://"+old+"#Old")
	if s.Address != "3.3.3.3" || s.Port != 1234 {
		t.Fatalf("ss legacy: %+v", s)
	}
}

func TestHysteria2(t *testing.T) {
	s := mustParse(t, "hy2://pw@hy.example.com:8443?sni=hy.example.com&obfs=salamander&obfs-password=ob&insecure=1#Hy")
	if s.Outbound["password"] != "pw" || jget(s.Outbound, "obfs", "password") != "ob" || jget(s.Outbound, "tls", "insecure") != true {
		t.Fatalf("hy2: %v", s.Outbound)
	}
}

func TestStableID(t *testing.T) {
	a := mustParse(t, "vless://u@h.com:443?security=tls&sni=h.com#Name1")
	b := mustParse(t, "vless://u@h.com:443?sni=h.com&security=tls#Другое имя")
	if a.ID != b.ID {
		t.Fatal("id должен зависеть только от параметров подключения")
	}
	c := mustParse(t, "vless://u@h.com:8443?security=tls&sni=h.com#Name1")
	if a.ID == c.ID {
		t.Fatal("разные серверы — разные id")
	}
}

func TestParseSubscriptionBase64(t *testing.T) {
	list := strings.Join([]string{
		"vless://u@a.com:443?security=tls#A",
		"vless://u@a.com:443?security=tls#A-dup",
		"garbage line",
		"vless://u@b.com:443?type=xhttp#B",
		"trojan://p@c.com:443#C",
		"vless://broken",
	}, "\r\n")
	res, err := ParseSubscription([]byte(base64.StdEncoding.EncodeToString([]byte(list))))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Servers) != 3 {
		t.Fatalf("want 3 servers (dedupe), got %d", len(res.Servers))
	}
	if res.Skipped != 1 {
		t.Fatalf("skipped=%d", res.Skipped)
	}
}

func TestParseSubscriptionSingBoxJSON(t *testing.T) {
	js := `{"outbounds":[{"type":"selector","tag":"sel","outbounds":["a"]},
	{"type":"vless","tag":"🇳🇱 NL","server":"nl.example.com","server_port":443,"uuid":"u","tls":{"enabled":true,"reality":{"enabled":true,"public_key":"k"}}},
	{"type":"direct","tag":"direct"}]}`
	res, err := ParseSubscription([]byte(js))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Servers) != 1 || res.Servers[0].Name != "🇳🇱 NL" || res.Servers[0].Security != "reality" {
		t.Fatalf("%+v", res.Servers)
	}
}

func TestBuildConfig(t *testing.T) {
	a := mustParse(t, "vless://u@a.com:443?security=tls#A")
	b := mustParse(t, "trojan://p@c.com:443#C")
	cfg, err := buildConfig([]ApplyServer{{a.ID, a.Outbound}, {b.ID, b.Outbound}}, b.ID, 2080, 9090, "sec")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	json.Unmarshal(cfg, &m)
	obs := m["outbounds"].([]any)
	sel := obs[0].(map[string]any)
	if sel["default"] != b.ID || len(sel["outbounds"].([]any)) != 2 {
		t.Fatalf("selector: %v", sel)
	}
	if obs[len(obs)-1].(map[string]any)["type"] != "direct" {
		t.Fatal("direct last")
	}
}
