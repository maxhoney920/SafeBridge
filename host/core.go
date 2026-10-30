package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	balancerTag = "proxy"
	coreName    = "xray"
)

func exeName(n string) string {
	if runtime.GOOS == "windows" {
		return n + ".exe"
	}
	return n
}

// ---------- бинарник Xray ----------

var (
	coreMu      sync.Mutex
	corePath    string
	corePathErr error
	coreVer     string
)

// CorePath возвращает путь к активному ядру Xray: скачанное обновление (core/active.json),
// встроенное в помощник или найденное рядом с программой / в PATH (для разработки).
func CorePath() (string, error) {
	coreMu.Lock()
	defer coreMu.Unlock()
	if corePath == "" && corePathErr == nil {
		corePath, corePathErr = resolveCore()
	}
	return corePath, corePathErr
}

// resetCoreCache — после обновления или отката ядра.
func resetCoreCache() {
	coreMu.Lock()
	corePath, corePathErr, coreVer = "", nil, ""
	coreMu.Unlock()
}

func coreDir() string { return filepath.Join(dataDir(), "core") }

type activeCore struct {
	Path    string `json:"path"`
	Version string `json:"version"`
}

func readActive() *activeCore {
	b, err := os.ReadFile(filepath.Join(coreDir(), "active.json"))
	if err != nil {
		return nil
	}
	var a activeCore
	if json.Unmarshal(b, &a) != nil || a.Path == "" {
		return nil
	}
	if st, err := os.Stat(a.Path); err != nil || st.IsDir() {
		return nil
	}
	return &a
}

func resolveCore() (string, error) {
	embedded, embErr := extractEmbedded()
	if a := readActive(); a != nil {
		// скачанное ядро используется, только если оно не старше встроенного
		if embedded == "" || compareVersions(a.Version, bundledXray) >= 0 {
			return a.Path, nil
		}
	}
	if embedded != "" {
		return embedded, nil
	}
	if embErr != nil {
		return "", embErr
	}
	var cands []string
	if exe, err := os.Executable(); err == nil {
		cands = append(cands, filepath.Join(filepath.Dir(exe), exeName(coreName)))
	}
	cands = append(cands, filepath.Join(coreDir(), exeName(coreName)))
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	if p, err := exec.LookPath(coreName); err == nil {
		return p, nil
	}
	return "", errors.New("ядро Xray не найдено (эта сборка помощника без встроенного ядра)")
}

// extractEmbedded распаковывает встроенное ядро (один раз на версию помощника).
func extractEmbedded() (string, error) {
	if len(embeddedCore) == 0 {
		return "", nil
	}
	sum := sha256.Sum256(embeddedCore)
	dir := coreDir()
	p := filepath.Join(dir, exeName(coreName+"-"+hex.EncodeToString(sum[:4])))
	if st, err := os.Stat(p); err == nil && st.Size() > 0 {
		return p, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	zr, err := gzip.NewReader(bytes.NewReader(embeddedCore))
	if err != nil {
		return "", fmt.Errorf("встроенное ядро повреждено: %v", err)
	}
	if err := writeExecutable(p, zr); err != nil {
		return "", err
	}
	// удаляем прежние встроенные ядра (и sing-box от первых версий помощника)
	for _, pat := range []string{coreName + "-????????" + exeName(""), "sing-box-*"} {
		old, _ := filepath.Glob(filepath.Join(dir, pat))
		for _, o := range old {
			if o != p {
				os.Remove(o)
			}
		}
	}
	return p, nil
}

func writeExecutable(p string, r io.Reader) error {
	tmp := p + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// binaryVersion запускает `xray version` и возвращает номер версии.
func binaryVersion(p string) (string, error) {
	cmd := exec.Command(p, "version")
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	// "Xray 26.9.30 (Xray, Penetrates Everything.) ..."
	f := strings.Fields(strings.SplitN(string(out), "\n", 2)[0])
	if len(f) < 2 || f[0] != "Xray" {
		return "", errors.New("это не Xray")
	}
	return f[1], nil
}

func CoreVersion() string {
	coreMu.Lock()
	v := coreVer
	coreMu.Unlock()
	if v != "" {
		return v
	}
	p, err := CorePath()
	if err != nil {
		return ""
	}
	v, _ = binaryVersion(p)
	coreMu.Lock()
	coreVer = v
	coreMu.Unlock()
	return v
}

// ---------- конфиг ----------

type ApplyServer struct {
	ID       string         `json:"id"`
	Outbound map[string]any `json:"outbound"`
}

// частные сети всегда идут напрямую; списком, чтобы не зависеть от geoip.dat
var privateCIDRs = []string{"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16",
	"172.16.0.0/12", "192.168.0.0/16", "224.0.0.0/4", "::1/128", "fc00::/7", "fe80::/10"}

type coreLayout struct {
	port     int
	testPort map[string]int // сервер → локальный SOCKS-порт для пинга
	tags     []string
	selected string
}

func validServers(servers []ApplyServer) []ApplyServer {
	var out []ApplyServer
	seen := map[string]bool{}
	for _, s := range servers {
		if s.ID == "" || s.Outbound == nil || seen[s.ID] || s.ID == balancerTag || s.ID == "direct" || s.ID == "api" {
			continue
		}
		seen[s.ID] = true
		out = append(out, s)
	}
	return out
}

func buildConfig(servers []ApplyServer, l *coreLayout) ([]byte, error) {
	servers = validServers(servers)
	if len(servers) == 0 {
		return nil, errors.New("нет поддерживаемых серверов")
	}
	l.tags = nil
	for _, s := range servers {
		l.tags = append(l.tags, s.ID)
	}
	if !contains(l.tags, l.selected) {
		l.selected = l.tags[0]
	}
	inbounds := []any{
		map[string]any{"tag": "main", "listen": "127.0.0.1", "port": l.port, "protocol": "socks",
			"settings": map[string]any{"auth": "noauth", "udp": true}},
	}
	rules := []any{}
	var outbounds []any
	for _, s := range servers {
		ob := sanitizeOutbound(s.Outbound)
		ob["tag"] = s.ID
		outbounds = append(outbounds, ob)
		if tp := l.testPort[s.ID]; tp > 0 {
			in := "t-" + s.ID
			inbounds = append(inbounds, map[string]any{"tag": in, "listen": "127.0.0.1", "port": tp, "protocol": "socks",
				"settings": map[string]any{"auth": "noauth"}})
			rules = append(rules, map[string]any{"inboundTag": []string{in}, "outboundTag": s.ID})
		}
	}
	outbounds = append(outbounds, map[string]any{"tag": "direct", "protocol": "freedom"})
	rules = append(rules,
		map[string]any{"ip": privateCIDRs, "outboundTag": "direct"},
		map[string]any{"domain": []string{"localhost"}, "outboundTag": "direct"},
		map[string]any{"inboundTag": []string{"main"}, "outboundTag": l.selected},
	)
	cfg := map[string]any{
		"log":       map[string]any{"loglevel": "warning", "access": "none", "dnsLog": false},
		"inbounds":  inbounds,
		"outbounds": outbounds,
		"routing": map[string]any{
			"domainStrategy": "AsIs",
			"rules":          rules,
		},
	}
	return json.MarshalIndent(cfg, "", "  ")
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// ---------- процесс ----------

type Core struct {
	mu       sync.Mutex
	cmd      *exec.Cmd
	done     chan struct{}
	layout   coreLayout
	hash     string
	stopping bool
	last     *applyArgs // последний набор серверов — для перезапуска после обновления ядра
	logs     *ring
	onExit   func(msg string)
}

type CoreStatus struct {
	Running  bool   `json:"running"`
	Port     int    `json:"port"`
	Selected string `json:"selected,omitempty"`
}

func NewCore(onExit func(string)) *Core {
	return &Core{logs: newRing(300), onExit: onExit}
}

func (c *Core) Status() CoreStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CoreStatus{Running: c.cmd != nil, Port: c.layout.port, Selected: c.layout.selected}
}

func (c *Core) LogTail(n int) []string { return c.logs.tail(n) }

func portFree(p int) bool {
	if p <= 0 || p > 65535 {
		return false
	}
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(p))
	if err != nil {
		return false
	}
	l.Close()
	return true
}

// freePorts занимает n свободных портов одновременно, чтобы они не совпали.
func freePorts(n int) ([]int, error) {
	var ls []net.Listener
	defer func() {
		for _, l := range ls {
			l.Close()
		}
	}()
	var out []int
	for i := 0; i < n; i++ {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		ls = append(ls, l)
		out = append(out, l.Addr().(*net.TCPAddr).Port)
	}
	return out, nil
}

// Apply запускает (или перезапускает) ядро с заданным набором серверов.
// Если набор не изменился — только переключает выбранный сервер, без перезапуска.
type applyArgs struct {
	servers       []ApplyServer
	selected      string
	preferredPort int
}

func (c *Core) Apply(servers []ApplyServer, selected string, preferredPort int) (CoreStatus, error) {
	servers = validServers(servers)
	c.mu.Lock()
	c.last = &applyArgs{servers, selected, preferredPort}
	c.mu.Unlock()
	return c.apply(servers, selected, preferredPort, false)
}

// Restart перезапускает ядро с последним набором серверов (например, после обновления Xray).
func (c *Core) Restart() (CoreStatus, error) {
	c.mu.Lock()
	last := c.last
	running := c.cmd != nil
	sel := c.layout.selected
	c.mu.Unlock()
	if last == nil || !running {
		return c.Status(), nil
	}
	return c.apply(last.servers, sel, last.preferredPort, true)
}

func (c *Core) apply(servers []ApplyServer, selected string, preferredPort int, force bool) (CoreStatus, error) {
	hashBytes, _ := json.Marshal(servers)
	h := sha256.Sum256(hashBytes)
	hash := hex.EncodeToString(h[:])

	c.mu.Lock()
	running := c.cmd != nil
	same := !force && running && c.hash == hash && (preferredPort <= 0 || preferredPort == c.layout.port)
	sameSel := c.layout.selected == selected || !contains(c.layout.tags, selected)
	prevPort := c.layout.port
	c.mu.Unlock()
	if same && sameSel {
		return c.Status(), nil
	}
	if len(servers) == 0 {
		return c.Status(), errors.New("нет поддерживаемых серверов")
	}
	corePath, err := CorePath()
	if err != nil {
		return c.Status(), err
	}
	c.Stop()

	port := 0
	switch {
	case preferredPort > 0 && portFree(preferredPort):
		port = preferredPort
	case prevPort > 0 && portFree(prevPort):
		port = prevPort
	}
	need := len(servers)
	if port == 0 {
		need++
	}
	ports, err := freePorts(need)
	if err != nil {
		return c.Status(), err
	}
	if port == 0 {
		port, ports = ports[0], ports[1:]
	}
	l := coreLayout{port: port, testPort: map[string]int{}, selected: selected}
	for i, s := range servers {
		l.testPort[s.ID] = ports[i]
	}
	cfg, err := buildConfig(servers, &l)
	if err != nil {
		return c.Status(), err
	}
	dir := dataDir()
	os.MkdirAll(dir, 0o700)
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, cfg, 0o600); err != nil {
		return c.Status(), err
	}

	killStaleCore(filepath.Join(dir, "core.pid"))

	cmd := exec.Command(corePath, "run", "-c", cfgPath)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "XRAY_LOCATION_ASSET="+dir)
	prepareChild(cmd)
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw
	logFile, _ := os.OpenFile(filepath.Join(dir, "core.log"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	c.logs.reset()
	go func() {
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			c.logs.add(line)
			if logFile != nil {
				logFile.WriteString(line + "\n")
			}
		}
		if logFile != nil {
			logFile.Close()
		}
	}()
	if err := cmd.Start(); err != nil {
		pw.Close()
		return c.Status(), fmt.Errorf("не удалось запустить Xray: %v", err)
	}
	afterStart(cmd)
	os.WriteFile(filepath.Join(dir, "core.pid"), []byte(strconv.Itoa(cmd.Process.Pid)), 0o600)

	done := make(chan struct{})
	c.mu.Lock()
	c.cmd, c.done, c.layout, c.hash, c.stopping = cmd, done, l, hash, false
	c.mu.Unlock()

	go func() {
		err := cmd.Wait()
		pw.Close()
		close(done)
		c.mu.Lock()
		intentional := c.stopping
		if c.cmd == cmd {
			c.cmd = nil
			c.hash = ""
		}
		c.mu.Unlock()
		os.Remove(filepath.Join(dir, "core.pid"))
		if !intentional && c.onExit != nil {
			msg := "ядро Xray завершилось"
			if err != nil {
				msg += ": " + err.Error()
			}
			c.onExit(msg)
		}
	}()

	// ждём, пока ядро начнёт принимать соединения на основном порту
	deadline := time.Now().Add(10 * time.Second)
	addr := "127.0.0.1:" + strconv.Itoa(l.port)
	for {
		select {
		case <-done:
			time.Sleep(100 * time.Millisecond)
			return c.Status(), fmt.Errorf("Xray не запустился:\n%s", strings.Join(c.logs.tail(15), "\n"))
		default:
		}
		if conn, err := net.DialTimeout("tcp", addr, 300*time.Millisecond); err == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			c.Stop()
			return c.Status(), errors.New("Xray не ответил за 10 секунд")
		}
		time.Sleep(50 * time.Millisecond)
	}
	return c.Status(), nil
}

func (c *Core) Stop() {
	c.mu.Lock()
	cmd, done := c.cmd, c.done
	if cmd != nil {
		c.stopping = true
	}
	c.mu.Unlock()
	if cmd == nil {
		return
	}
	terminate(cmd)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		cmd.Process.Kill()
		<-done
	}
}

// Select переключает сервер. Управляющего API у ядра нет (через него любая программа на
// компьютере могла бы менять маршруты), поэтому ядро перезапускается с новым конфигом.
func (c *Core) Select(id string) error {
	if id == "" {
		return errors.New("сервер не указан")
	}
	c.mu.Lock()
	running := c.cmd != nil
	known := contains(c.layout.tags, id)
	last := c.last
	c.mu.Unlock()
	if !running || last == nil {
		return errors.New("ядро не запущено")
	}
	if !known {
		return errors.New("сервер не найден в текущем наборе")
	}
	c.mu.Lock()
	c.last = &applyArgs{last.servers, id, last.preferredPort}
	c.mu.Unlock()
	_, err := c.apply(last.servers, id, last.preferredPort, true)
	return err
}

// Delay измеряет время ответа testURL через конкретный сервер (через его тестовый SOCKS-порт).
func (c *Core) Delay(id, testURL string, timeoutMs int) int {
	c.mu.Lock()
	tp := c.layout.testPort[id]
	c.mu.Unlock()
	if tp == 0 {
		return -1
	}
	proxy, _ := url.Parse("socks5://127.0.0.1:" + strconv.Itoa(tp))
	tr := &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true, ForceAttemptHTTP2: false}
	defer tr.CloseIdleConnections()
	cl := &http.Client{Transport: tr, Timeout: time.Duration(timeoutMs) * time.Millisecond,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequest(http.MethodGet, testURL, nil)
	if err != nil {
		return -1
	}
	start := time.Now()
	resp, err := cl.Do(req)
	if err != nil {
		return -1
	}
	resp.Body.Close()
	d := int(time.Since(start).Milliseconds())
	if d < 1 {
		d = 1
	}
	return d
}

func (c *Core) DelayMany(ctx context.Context, ids []string, testURL string, timeoutMs int) map[string]int {
	res := map[string]int{}
	var mu sync.Mutex
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			d := c.Delay(id, testURL, timeoutMs)
			<-sem
			mu.Lock()
			res[id] = d
			mu.Unlock()
		}(id)
	}
	wg.Wait()
	return res
}

// ---------- кольцевой буфер логов ----------

type ring struct {
	mu    sync.Mutex
	lines []string
	max   int
}

func newRing(n int) *ring { return &ring{max: n} }

func (r *ring) add(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, s)
	if len(r.lines) > r.max {
		r.lines = r.lines[len(r.lines)-r.max:]
	}
}

func (r *ring) reset() {
	r.mu.Lock()
	r.lines = nil
	r.mu.Unlock()
}

func (r *ring) tail(n int) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n > len(r.lines) {
		n = len(r.lines)
	}
	out := make([]string, n)
	copy(out, r.lines[len(r.lines)-n:])
	return out
}
