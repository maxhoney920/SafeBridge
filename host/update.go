package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Задаются при сборке (-ldflags "-X main.updateBase=... -X main.bundledXray=...").
var (
	// откуда брать latest.json и новые версии помощника (ветка downloads репозитория)
	updateBase = "https://raw.githubusercontent.com/maxhoney920/SafeBridge/downloads/"
	// версия Xray, встроенная в эту сборку помощника
	bundledXray = ""
)

const xrayRepo = "XTLS/Xray-core"

// ---------- сравнение версий ----------

// compareVersions сравнивает "26.9.30" и "v26.10.21"; пустая или "dev" считается самой старой.
func compareVersions(a, b string) int {
	pa, pb := versionParts(a), versionParts(b)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func versionParts(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+ "); i >= 0 {
		v = v[:i]
	}
	var out []int
	for _, p := range strings.Split(v, ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out
		}
		out = append(out, n)
	}
	return out
}

// ---------- загрузка ----------

// download скачивает URL напрямую, а если не вышло и ядро запущено — через локальный прокси.
func download(ctx context.Context, u string, limit int64, proxyPort int) ([]byte, error) {
	b, err := downloadVia(ctx, u, limit, nil)
	if err != nil && proxyPort > 0 {
		p, _ := url.Parse("socks5://127.0.0.1:" + strconv.Itoa(proxyPort))
		if b2, err2 := downloadVia(ctx, u, limit, p); err2 == nil {
			return b2, nil
		}
	}
	return b, err
}

func downloadVia(ctx context.Context, u string, limit int64, proxy *url.URL) ([]byte, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if proxy != nil {
		tr.Proxy = http.ProxyURL(proxy)
	}
	cl := &http.Client{Transport: tr, Timeout: 5 * time.Minute}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "SafeBridge/"+version)
	resp, err := cl.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", u, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("файл слишком большой")
	}
	return b, nil
}

func sha256hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// ---------- что нового ----------

type latestInfo struct {
	Version string                    `json:"version"`
	Xray    string                    `json:"xray"`
	Files   map[string]latestInfoFile `json:"files"`
}

type latestInfoFile struct {
	Name   string `json:"name"`
	Sha256 string `json:"sha256"`
}

type UpdateInfo struct {
	Helper struct {
		Current   string `json:"current"`
		Latest    string `json:"latest,omitempty"`
		Available bool   `json:"available"`
		Error     string `json:"error,omitempty"`
	} `json:"helper"`
	Core struct {
		Current   string `json:"current"`
		Latest    string `json:"latest,omitempty"`
		Available bool   `json:"available"`
		Error     string `json:"error,omitempty"`
	} `json:"core"`
}

func platformKey() string { return runtime.GOOS + "-" + runtime.GOARCH }

func fetchLatestHelper(ctx context.Context, proxyPort int) (*latestInfo, error) {
	b, err := download(ctx, updateBase+"latest.json?t="+strconv.FormatInt(time.Now().Unix(), 10), 1<<20, proxyPort)
	if err != nil {
		return nil, err
	}
	var li latestInfo
	if err := json.Unmarshal(b, &li); err != nil {
		return nil, errors.New("latest.json повреждён")
	}
	return &li, nil
}

// latestXray узнаёт последнюю стабильную версию Xray: через API GitHub,
// а если он недоступен (лимит запросов) — по редиректу страницы releases/latest.
func latestXray(ctx context.Context, proxyPort int) (string, error) {
	if b, err := download(ctx, "https://api.github.com/repos/"+xrayRepo+"/releases/latest", 1<<20, proxyPort); err == nil {
		var r struct {
			Tag string `json:"tag_name"`
		}
		if json.Unmarshal(b, &r) == nil && r.Tag != "" {
			return strings.TrimPrefix(r.Tag, "v"), nil
		}
	}
	cl := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://github.com/"+xrayRepo+"/releases/latest", nil)
	resp, err := cl.Do(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	if i := strings.LastIndex(loc, "/tag/"); i >= 0 {
		return strings.TrimPrefix(loc[i+5:], "v"), nil
	}
	return "", errors.New("не удалось узнать последнюю версию Xray")
}

func CheckUpdates(ctx context.Context, proxyPort int) *UpdateInfo {
	ui := &UpdateInfo{}
	ui.Helper.Current = version
	ui.Core.Current = CoreVersion()
	if li, err := fetchLatestHelper(ctx, proxyPort); err != nil {
		ui.Helper.Error = err.Error()
	} else {
		ui.Helper.Latest = li.Version
		_, hasFile := li.Files[platformKey()]
		ui.Helper.Available = hasFile && version != "dev" && compareVersions(li.Version, version) > 0
	}
	if v, err := latestXray(ctx, proxyPort); err != nil {
		ui.Core.Error = err.Error()
	} else {
		ui.Core.Latest = v
		ui.Core.Available = compareVersions(v, ui.Core.Current) > 0
	}
	return ui
}

// ---------- обновление ядра Xray ----------

func xrayAssetName() (string, error) {
	osName := map[string]string{"windows": "windows", "darwin": "macos", "linux": "linux"}[runtime.GOOS]
	arch := map[string]string{"amd64": "64", "arm64": "arm64-v8a"}[runtime.GOARCH]
	if osName == "" || arch == "" {
		return "", errors.New("для этой платформы обновление ядра не поддерживается")
	}
	return "Xray-" + osName + "-" + arch + ".zip", nil
}

// UpdateCore скачивает Xray нужной версии из официальных релизов, сверяет SHA-256,
// проверяет текущий конфиг новым ядром и только после этого переключается на него.
// Если ядро с новой версией не запускается — возвращает прежнее.
func (c *Core) UpdateCore(ctx context.Context, ver string, proxyPort int) (map[string]any, error) {
	var err error
	if ver == "" {
		if ver, err = latestXray(ctx, proxyPort); err != nil {
			return nil, err
		}
	}
	ver = strings.TrimPrefix(ver, "v")
	cur := CoreVersion()
	if compareVersions(ver, cur) <= 0 {
		return map[string]any{"updated": false, "version": cur}, nil
	}
	asset, err := xrayAssetName()
	if err != nil {
		return nil, err
	}
	base := "https://github.com/" + xrayRepo + "/releases/download/v" + ver + "/" + asset
	dgst, err := download(ctx, base+".dgst", 64<<10, proxyPort)
	if err != nil {
		return nil, fmt.Errorf("не удалось скачать контрольную сумму: %v", err)
	}
	want := ""
	for _, line := range strings.Split(string(dgst), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok && strings.EqualFold(strings.TrimSpace(k), "SHA2-256") {
			want = strings.ToLower(strings.TrimSpace(v))
		}
	}
	if want == "" {
		return nil, errors.New("в файле контрольных сумм нет SHA2-256")
	}
	zb, err := download(ctx, base, 100<<20, proxyPort)
	if err != nil {
		return nil, fmt.Errorf("не удалось скачать Xray %s: %v", ver, err)
	}
	if got := sha256hex(zb); got != want {
		return nil, errors.New("контрольная сумма архива не совпала — обновление отменено")
	}
	zr, err := zip.NewReader(bytes.NewReader(zb), int64(len(zb)))
	if err != nil {
		return nil, err
	}
	var bin *zip.File
	for _, f := range zr.File {
		if f.Name == exeName("xray") {
			bin = f
		}
	}
	if bin == nil {
		return nil, errors.New("в архиве нет исполняемого файла xray")
	}
	rc, err := bin.Open()
	if err != nil {
		return nil, err
	}
	dst := filepath.Join(coreDir(), exeName("xray-v"+ver))
	os.MkdirAll(coreDir(), 0o755)
	err = writeExecutable(dst, rc)
	rc.Close()
	if err != nil {
		return nil, err
	}
	if got, err := binaryVersion(dst); err != nil || compareVersions(got, ver) != 0 {
		os.Remove(dst)
		return nil, errors.New("скачанное ядро не запускается на этом компьютере")
	}
	// текущий конфиг должен приниматься новым ядром
	cfgPath := filepath.Join(dataDir(), "config.json")
	if _, err := os.Stat(cfgPath); err == nil {
		cmd := exec.Command(dst, "run", "-test", "-c", cfgPath)
		hideWindow(cmd)
		if out, err := cmd.CombinedOutput(); err != nil {
			os.Remove(dst)
			return nil, fmt.Errorf("Xray %s не принимает текущие серверы, обновление отменено: %s", ver, lastLine(string(out)))
		}
	}

	prev, _ := os.ReadFile(filepath.Join(coreDir(), "active.json"))
	if err := writeActive(activeCore{Path: dst, Version: ver}); err != nil {
		return nil, err
	}
	resetCoreCache()
	if _, err := c.Restart(); err != nil {
		// откат на прежнее ядро
		if len(prev) > 0 {
			os.WriteFile(filepath.Join(coreDir(), "active.json"), prev, 0o644)
		} else {
			os.Remove(filepath.Join(coreDir(), "active.json"))
		}
		resetCoreCache()
		c.Restart()
		os.Remove(dst)
		return nil, fmt.Errorf("Xray %s не запустился, возвращена версия %s: %v", ver, cur, err)
	}
	cleanupCores(dst)
	return map[string]any{"updated": true, "version": ver, "previous": cur}, nil
}

func writeActive(a activeCore) error {
	b, _ := json.MarshalIndent(a, "", "  ")
	p := filepath.Join(coreDir(), "active.json")
	if err := os.WriteFile(p+".tmp", b, 0o644); err != nil {
		return err
	}
	return os.Rename(p+".tmp", p)
}

// cleanupCores удаляет скачанные ядра, кроме активного.
func cleanupCores(keep string) {
	old, _ := filepath.Glob(filepath.Join(coreDir(), "xray-v*"))
	for _, o := range old {
		if o != keep {
			os.Remove(o)
		}
	}
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// ---------- обновление самого помощника ----------

// UpdateHelper скачивает новую версию помощника из ветки downloads, сверяет SHA-256
// и подменяет исполняемый файл. После ответа помощник завершается — браузер
// запустит новую версию при следующем подключении расширения.
func UpdateHelper(ctx context.Context, proxyPort int) (map[string]any, error) {
	li, err := fetchLatestHelper(ctx, proxyPort)
	if err != nil {
		return nil, err
	}
	if version != "dev" && compareVersions(li.Version, version) <= 0 {
		return map[string]any{"updated": false, "version": version}, nil
	}
	f, ok := li.Files[platformKey()]
	if !ok || f.Name == "" || f.Sha256 == "" {
		return nil, errors.New("для этой платформы обновление не опубликовано")
	}
	// параметр в адресе обходит кэш GitHub: сразу после публикации он ещё несколько минут
	// может отдавать прежний файл, и его хеш не совпадёт со свежим latest.json
	var b []byte
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(3 * time.Second):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		u := updateBase + f.Name + "?v=" + url.QueryEscape(li.Version) + "&t=" + strconv.FormatInt(time.Now().UnixNano(), 10)
		b, err = download(ctx, u, 200<<20, proxyPort)
		if err != nil {
			return nil, err
		}
		if sha256hex(b) == strings.ToLower(f.Sha256) {
			break
		}
		b = nil
	}
	if b == nil {
		return nil, errors.New("контрольная сумма не совпала — обновление отменено. Если версия вышла только что, попробуйте через 5 минут")
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	tmp := strings.TrimSuffix(exe, ".exe") + ".new" + exeName("")
	if err := os.WriteFile(tmp, b, 0o755); err != nil {
		return nil, err
	}
	clearQuarantine(tmp) // SHA-256 уже сверен; без этого macOS не даст запустить файл
	check := exec.Command(tmp, "version")
	hideWindow(check)
	if v, err := check.Output(); err != nil || !strings.Contains(string(v), li.Version) {
		os.Remove(tmp)
		return nil, errors.New("новая версия помощника не запускается на этом компьютере")
	}
	if runtime.GOOS == "windows" {
		// запущенный exe нельзя перезаписать, но можно переименовать
		old := exe + ".old"
		os.Remove(old)
		if err := os.Rename(exe, old); err != nil {
			os.Remove(tmp)
			return nil, err
		}
	}
	if err := os.Rename(tmp, exe); err != nil {
		return nil, err
	}
	// новая версия может знать новые браузеры (например, Firefox) — перерегистрируемся ею
	reg := exec.Command(exe, "install")
	hideWindow(reg)
	reg.Run()
	return map[string]any{"updated": true, "version": li.Version, "previous": version}, nil
}

// cleanupOldHelper удаляет файл, оставшийся от обновления на Windows.
func cleanupOldHelper() {
	if exe, err := os.Executable(); err == nil {
		os.Remove(exe + ".old")
		os.Remove(strings.TrimSuffix(exe, ".exe") + ".new" + exeName(""))
	}
}
