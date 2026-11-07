package host

import (
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Обновление самого приложения — как у помощника расширения: версия и SHA-256 файлов
// берутся из подписанного latest.json (поле app), файл скачивается из ветки downloads и
// ставится только при совпадении хеша. Выполняется в процессе окна (не в службе).

// AppInfo — что известно о новой версии приложения.
type AppInfo struct {
	Current   string `json:"current"`
	Latest    string `json:"latest,omitempty"`
	Available bool   `json:"available"`
	CanUpdate bool   `json:"canUpdate"` // можно обновить из приложения (иначе — скачать вручную)
	Pending   bool   `json:"pending,omitempty"` // новая версия вышла, но сборки для этой системы ещё нет
	Page      string `json:"page,omitempty"`
	Error     string `json:"error,omitempty"`
}

// appPlatformKey — ключ файла приложения в latest.json.
func appPlatformKey() string {
	switch {
	case isWin7Build():
		return "windows7-" + runtime.GOARCH
	case runtime.GOOS == "darwin" && isMacLegacy():
		return "macos-legacy"
	case runtime.GOOS == "darwin":
		return "macos"
	default:
		return runtime.GOOS + "-" + runtime.GOARCH
	}
}

// AppDownloadPage — где скачать приложение вручную.
func AppDownloadPage() string {
	return strings.TrimSuffix(strings.Replace(updateBase, "raw.githubusercontent.com", "github.com", 1), "/downloads/") + "/tree/downloads#приложение-для-компьютера"
}

func CheckApp(ctx context.Context, proxyPort int) *AppInfo {
	ai := &AppInfo{Current: version, Page: AppDownloadPage()}
	li, err := fetchLatestHelper(ctx, proxyPort)
	if err != nil {
		ai.Error = err.Error()
		return ai
	}
	if li.App == nil || li.App.Version == "" {
		return ai
	}
	ai.Latest = li.App.Version
	_, has := li.App.Files[appPlatformKey()]
	newer := version != "dev" && compareVersions(li.App.Version, version) > 0
	ai.Available = has && newer
	ai.Pending = newer && !has
	ai.CanUpdate = ai.Available && (runtime.GOOS == "windows" || runtime.GOOS == "darwin")
	return ai
}

// UpdateApp скачивает и ставит новую версию. Возвращает путь, который нужно запустить
// после выхода (новый exe или .app); перезапуск делает вызывающий.
func UpdateApp(ctx context.Context, proxyPort int) (string, string, error) {
	li, err := fetchLatestHelper(ctx, proxyPort)
	if err != nil {
		return "", "", err
	}
	if li.App == nil || compareVersions(li.App.Version, version) <= 0 {
		return "", "", errors.New("установлена последняя версия")
	}
	f, ok := li.App.Files[appPlatformKey()]
	if !ok || f.Name == "" || f.Sha256 == "" {
		return "", "", errors.New("для этой системы обновление не опубликовано")
	}
	var b []byte
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(3 * time.Second):
			case <-ctx.Done():
				return "", "", ctx.Err()
			}
		}
		u := updateBase + f.Name + "?v=" + url.QueryEscape(li.App.Version) + "&t=" + strconv.FormatInt(time.Now().UnixNano(), 10)
		if b, err = download(ctx, u, 300<<20, proxyPort); err != nil {
			return "", "", err
		}
		if sha256hex(b) == strings.ToLower(f.Sha256) {
			break
		}
		b = nil
	}
	if b == nil {
		return "", "", errors.New("контрольная сумма не совпала — обновление отменено. Если версия вышла только что, попробуйте через 5 минут")
	}
	exe, err := os.Executable()
	if err != nil {
		return "", "", err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	switch runtime.GOOS {
	case "windows":
		p, err := replaceExe(exe, b, li.App.Version)
		return p, li.App.Version, err
	case "darwin":
		p, err := replaceBundle(exe, b, li.App.Version)
		return p, li.App.Version, err
	}
	return "", "", errors.New("на этой системе обновите приложение вручную")
}

// checkRuns — новая версия запускается и называет нужную версию (--version).
func checkRuns(path, want string) error {
	c := exec.Command(path, "--version")
	hideWindow(c)
	out, err := c.Output()
	if err != nil || !strings.Contains(string(out), want) {
		return errors.New("новая версия не запускается на этом компьютере — обновление отменено")
	}
	return nil
}

func replaceExe(exe string, b []byte, ver string) (string, error) {
	tmp := strings.TrimSuffix(exe, ".exe") + ".new.exe"
	if err := os.WriteFile(tmp, b, 0o755); err != nil {
		return "", errors.New("не удалось записать новую версию рядом с программой: " + err.Error())
	}
	if err := checkRuns(tmp, ver); err != nil {
		os.Remove(tmp)
		return "", err
	}
	old := exe + ".old"
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil { // запущенный exe можно переименовать
		os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, exe); err != nil {
		os.Rename(old, exe)
		return "", err
	}
	return exe, nil
}

// replaceBundle заменяет SafeBridge.app целиком: архив распаковывается рядом, старая
// версия убирается, новая встаёт на её место.
func replaceBundle(exe string, zip []byte, ver string) (string, error) {
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(exe))) // …/SafeBridge.app/Contents/MacOS/SafeBridge
	if !strings.HasSuffix(bundle, ".app") {
		return "", errors.New("SafeBridge запущен не из SafeBridge.app — обновите вручную")
	}
	parent := filepath.Dir(bundle)
	work, err := os.MkdirTemp(parent, ".safebridge-update-")
	if err != nil {
		return "", errors.New("нет прав на запись в папку " + parent + " — скачайте новую версию вручную")
	}
	defer os.RemoveAll(work)
	zp := filepath.Join(work, "update.zip")
	if err := os.WriteFile(zp, zip, 0o600); err != nil {
		return "", err
	}
	if out, err := exec.Command("ditto", "-x", "-k", zp, work).CombinedOutput(); err != nil {
		return "", errors.New("не удалось распаковать обновление: " + strings.TrimSpace(string(out)))
	}
	fresh := filepath.Join(work, "SafeBridge.app")
	if err := checkRuns(filepath.Join(fresh, "Contents", "MacOS", "SafeBridge"), ver); err != nil {
		return "", err
	}
	exec.Command("xattr", "-cr", fresh).Run() // SHA-256 сверен; без этого Gatekeeper спросит снова
	old := filepath.Join(work, "old.app")
	if err := os.Rename(bundle, old); err != nil {
		return "", errors.New("не удалось заменить приложение: " + err.Error())
	}
	if err := os.Rename(fresh, bundle); err != nil {
		os.Rename(old, bundle)
		return "", errors.New("не удалось заменить приложение: " + err.Error())
	}
	return bundle, nil
}

// CleanupAppUpdate убирает файл, оставшийся от обновления на Windows.
func CleanupAppUpdate() {
	if exe, err := os.Executable(); err == nil {
		os.Remove(exe + ".old")
		os.Remove(strings.TrimSuffix(exe, ".exe") + ".new.exe")
	}
}
