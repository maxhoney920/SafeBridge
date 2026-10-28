package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
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

const selectorTag = "proxy"

func exeName(n string) string {
	if runtime.GOOS == "windows" {
		return n + ".exe"
	}
	return n
}

// ---------- бинарник sing-box ----------

var (
	corePathOnce sync.Once
	corePath     string
	corePathErr  error
	coreVerOnce  sync.Once
	coreVer      string
)

// CorePath возвращает путь к sing-box: распаковывает встроенный бинарник
// в каталог данных или ищет рядом с программой / в PATH (для разработки).
func CorePath() (string, error) {
	corePathOnce.Do(func() { corePath, corePathErr = resolveCore() })
	return corePath, corePathErr
}

func resolveCore() (string, error) {
	if len(embeddedCore) > 0 {
		sum := sha256.Sum256(embeddedCore)
		dir := filepath.Join(dataDir(), "core")
		name := exeName("sing-box-" + hex.EncodeToString(sum[:4]))
		p := filepath.Join(dir, name)
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
		tmp := p + ".tmp"
		f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(f, zr); err != nil {
			f.Close()
			os.Remove(tmp)
			return "", err
		}
		f.Close()
		if err := os.Rename(tmp, p); err != nil {
			return "", err
		}
		// удаляем старые версии ядра
		if old, _ := filepath.Glob(filepath.Join(dir, "sing-box-*")); len(old) > 0 {
			for _, o := range old {
				if o != p {
					os.Remove(o)
				}
			}
		}
		return p, nil
	}
	var cands []string
	if exe, err := os.Executable(); err == nil {
		cands = append(cands, filepath.Join(filepath.Dir(exe), exeName("sing-box")))
	}
	cands = append(cands, filepath.Join(dataDir(), "core", exeName("sing-box")))
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	if p, err := exec.LookPath("sing-box"); err == nil {
		return p, nil
	}
	return "", errors.New("ядро sing-box не найдено (эта сборка помощника без встроенного ядра)")
}

func CoreVersion() string {
	coreVerOnce.Do(func() {
		p, err := CorePath()
		if err != nil {
			return
		}
		cmd := exec.Command(p, "version")
		hideWindow(cmd)
		out, err := cmd.Output()
		if err != nil {
			return
		}
		line := strings.SplitN(string(out), "\n", 2)[0]
		coreVer = strings.TrimSpace(strings.TrimPrefix(line, "sing-box version"))
	})
	return coreVer
}

// ---------- конфиг ----------

type ApplyServer struct {
	ID       string         `json:"id"`
	Outbound map[string]any `json:"outbound"`
}

func buildConfig(servers []ApplyServer, selected string, port, apiPort int, secret string) ([]byte, error) {
	if len(servers) == 0 {
		return nil, errors.New("нет поддерживаемых серверов")
	}
	tags := make([]string, 0, len(servers))
	outbounds := []any{}
	seen := map[string]bool{}
	for _, s := range servers {
		if s.ID == "" || s.Outbound == nil || seen[s.ID] || s.ID == selectorTag || s.ID == "direct" {
			continue
		}
		seen[s.ID] = true
		ob := map[string]any{}
		for k, v := range s.Outbound {
			ob[k] = v
		}
		ob["tag"] = s.ID
		tags = append(tags, s.ID)
		outbounds = append(outbounds, ob)
	}
	if len(tags) == 0 {
		return nil, errors.New("нет поддерживаемых серверов")
	}
	if !seen[selected] {
		selected = tags[0]
	}
	sel := map[string]any{"type": "selector", "tag": selectorTag, "outbounds": tags, "default": selected,
		"interrupt_exist_connections": true}
	all := append([]any{sel}, outbounds...)
	all = append(all, map[string]any{"type": "direct", "tag": "direct"})
	cfg := map[string]any{
		"log": map[string]any{"level": "warn", "timestamp": true},
		"dns": map[string]any{"servers": []any{map[string]any{"type": "local", "tag": "local"}}},
		"inbounds": []any{map[string]any{"type": "mixed", "tag": "mixed-in", "listen": "127.0.0.1",
			"listen_port": port}},
		"outbounds": all,
		"route": map[string]any{
			"rules":                   []any{map[string]any{"ip_is_private": true, "outbound": "direct"}},
			"final":                   selectorTag,
			"default_domain_resolver": "local",
		},
		"experimental": map[string]any{"clash_api": map[string]any{
			"external_controller": "127.0.0.1:" + strconv.Itoa(apiPort), "secret": secret}},
	}
	return json.MarshalIndent(cfg, "", "  ")
}

// ---------- процесс ----------

type Core struct {
	mu       sync.Mutex
	cmd      *exec.Cmd
	done     chan struct{}
	port     int
	apiPort  int
	secret   string
	hash     string
	selected string
	stopping bool
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
	return CoreStatus{Running: c.cmd != nil, Port: c.port, Selected: c.selected}
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

func randomPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Apply запускает (или перезапускает) ядро с заданным набором серверов.
// Если набор не изменился — только переключает выбранный сервер, без перезапуска.
func (c *Core) Apply(servers []ApplyServer, selected string, preferredPort int) (CoreStatus, error) {
	hashBytes, _ := json.Marshal(servers)
	h := sha256.Sum256(hashBytes)
	hash := hex.EncodeToString(h[:])

	c.mu.Lock()
	running := c.cmd != nil
	same := running && c.hash == hash && (preferredPort <= 0 || preferredPort == c.port)
	c.mu.Unlock()
	if same {
		if err := c.Select(selected); err != nil {
			return c.Status(), err
		}
		return c.Status(), nil
	}

	// проверяем набор до остановки работающего ядра, чтобы ошибка не оставила пользователя без прокси
	if _, err := buildConfig(servers, selected, 1, 1, "x"); err != nil {
		return c.Status(), err
	}
	c.Stop()

	corePath, err := CorePath()
	if err != nil {
		return c.Status(), err
	}
	port := 0
	switch {
	case preferredPort > 0 && portFree(preferredPort):
		port = preferredPort
	case c.port > 0 && portFree(c.port):
		port = c.port
	default:
		if port, err = randomPort(); err != nil {
			return c.Status(), err
		}
	}
	apiPort, err := randomPort()
	if err != nil {
		return c.Status(), err
	}
	secret := randomHex(16)
	cfg, err := buildConfig(servers, selected, port, apiPort, secret)
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

	cmd := exec.Command(corePath, "run", "-c", cfgPath, "--disable-color")
	cmd.Dir = dir
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
		return c.Status(), fmt.Errorf("не удалось запустить sing-box: %v", err)
	}
	afterStart(cmd)
	os.WriteFile(filepath.Join(dir, "core.pid"), []byte(strconv.Itoa(cmd.Process.Pid)), 0o600)

	done := make(chan struct{})
	c.mu.Lock()
	c.cmd, c.done, c.port, c.apiPort, c.secret, c.hash, c.stopping = cmd, done, port, apiPort, secret, hash, false
	c.selected = ""
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
			msg := "ядро sing-box завершилось"
			if err != nil {
				msg += ": " + err.Error()
			}
			c.onExit(msg)
		}
	}()

	// ждём, пока поднимется API
	deadline := time.Now().Add(10 * time.Second)
	for {
		select {
		case <-done:
			time.Sleep(100 * time.Millisecond)
			return c.Status(), fmt.Errorf("sing-box не запустился:\n%s", strings.Join(c.logs.tail(15), "\n"))
		default:
		}
		if _, err := c.api(http.MethodGet, "/version", nil, 1*time.Second); err == nil {
			break
		}
		if time.Now().After(deadline) {
			c.Stop()
			return c.Status(), errors.New("sing-box не ответил за 10 секунд")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := c.Select(selected); err != nil {
		// выбранного сервера нет — остаётся сервер по умолчанию
		c.mu.Lock()
		c.selected = servers[0].ID
		c.mu.Unlock()
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

func (c *Core) api(method, path string, body any, timeout time.Duration) ([]byte, error) {
	c.mu.Lock()
	apiPort, secret := c.apiPort, c.secret
	c.mu.Unlock()
	if apiPort == 0 {
		return nil, errors.New("ядро не запущено")
	}
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, "http://127.0.0.1:"+strconv.Itoa(apiPort)+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	cl := &http.Client{Timeout: timeout, Transport: &http.Transport{Proxy: nil}}
	resp, err := cl.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		var m struct {
			Message string `json:"message"`
		}
		json.Unmarshal(b, &m)
		if m.Message == "" {
			m.Message = resp.Status
		}
		return b, errors.New(m.Message)
	}
	return b, nil
}

func (c *Core) Select(id string) error {
	if id == "" {
		return errors.New("сервер не указан")
	}
	if _, err := c.api(http.MethodPut, "/proxies/"+url.PathEscape(selectorTag), map[string]string{"name": id}, 3*time.Second); err != nil {
		return err
	}
	c.mu.Lock()
	c.selected = id
	c.mu.Unlock()
	return nil
}

// Delay измеряет задержку через конкретный сервер (Clash API URL-test).
func (c *Core) Delay(id, testURL string, timeoutMs int) int {
	path := "/proxies/" + url.PathEscape(id) + "/delay?timeout=" + strconv.Itoa(timeoutMs) + "&url=" + url.QueryEscape(testURL)
	b, err := c.api(http.MethodGet, path, nil, time.Duration(timeoutMs+2000)*time.Millisecond)
	if err != nil {
		return -1
	}
	var r struct {
		Delay int `json:"delay"`
	}
	if json.Unmarshal(b, &r) != nil || r.Delay <= 0 {
		return -1
	}
	return r.Delay
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
