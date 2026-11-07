package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/safebridge/desktop/internal/host"
	"github.com/safebridge/desktop/internal/svc"
	"github.com/safebridge/desktop/internal/sysproxy"
	"github.com/safebridge/desktop/internal/tray"
)

var version = "1.0.0"

// App — то, что видит интерфейс (тот же код, что у расширения) через мост:
// ядро вместо помощника, хранилище настроек, системный прокси, вход в аккаунт.
type App struct {
	ctx     context.Context
	core    svc.Core    // ядро: в этом процессе или в службе
	host    *host.Host  // ядро в этом процессе (нет, если ядро в службе)
	svcc    *svc.Client // связь со службой (macOS)
	tunOnly bool        // только режим «Весь трафик» (macOS): системный прокси не трогаем
	local   *localServer
	emit    func(name string, data string) // событие в интерфейс (Wails или режим разработки)
	ui      uiHooks

	mu       sync.Mutex
	pac      string // текущий PAC от интерфейса (с портами ядра)
	pacSeq   int
	proxyOn  bool
	trayLast string // последнее меню значка (для проверки без macOS)
}

// uiHooks — то, что зависит от оболочки (окно Wails или браузер в режиме разработки).
type uiHooks struct {
	openURL func(string)
	show    func()
	quit    func()
}

func NewApp() *App {
	a := &App{}
	sysproxy.Init(host.DataDir())
	sysproxy.Recover() // если прошлый запуск упал и не вернул настройки прокси
	if useService() {
		// macOS: ядро с правами администратора работает в службе, окно — от имени пользователя
		a.svcc = &svc.Client{Path: serviceSocket(), Emit: func(n, d string) { a.emit(n, d) }}
		a.core = a.svcc
		a.tunOnly = true
	} else {
		a.host = host.New(func(b []byte) { a.emit("native", string(b)) })
		a.core = a.host
	}
	a.local = startLocal(a)
	tray.OnClick = func(id string) { a.emit("tray", id) }
	return a
}

func (a *App) shutdown() {
	a.mu.Lock()
	on := a.proxyOn
	a.proxyOn = false
	a.mu.Unlock()
	if on {
		sysproxy.Clear()
	}
	tray.Remove()
	a.core.Shutdown()
}

// ---------- методы для интерфейса ----------

// Native — команда ядру ({id, cmd, args}), ответ придёт событием "native".
func (a *App) Native(msg string) { a.core.HandleJSON([]byte(msg)) }

func settingsPath() string { return filepath.Join(host.DataDir(), "settings.json") }

// StorageLoad — все настройки интерфейса (то, что расширение держит в storage.local).
func (a *App) StorageLoad() string {
	b, err := os.ReadFile(settingsPath())
	if err != nil || !json.Valid(b) {
		return "{}"
	}
	return string(b)
}

// StorageSave сохраняет настройки целиком (файл доступен только пользователю).
func (a *App) StorageSave(data string) error {
	if !json.Valid([]byte(data)) {
		return errors.New("некорректные настройки")
	}
	os.MkdirAll(host.DataDir(), 0o700)
	tmp := settingsPath() + ".tmp"
	if err := os.WriteFile(tmp, []byte(data), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, settingsPath())
}

var socksRe = regexp.MustCompile(`SOCKS5 127\.0\.0\.1:(\d+)`)

// SetProxy — то, что расширение передаёт в chrome.proxy.settings: PAC или «напрямую».
// В приложении PAC включается как системная автонастройка прокси. В режиме TUN
// системный прокси не нужен: весь трафик и так идёт через адаптер.
func (a *App) SetProxy(value string) error {
	var v struct {
		Mode      string `json:"mode"`
		PacScript struct {
			Data string `json:"data"`
		} `json:"pacScript"`
	}
	json.Unmarshal([]byte(value), &v)
	if a.tunOnly {
		return nil // весь трафик идёт через TUN-адаптер службы
	}
	st := a.host.Status()
	if v.Mode != "pac_script" || st.TUN {
		a.mu.Lock()
		on := a.proxyOn
		a.proxyOn = false
		a.mu.Unlock()
		if on {
			return sysproxy.Clear()
		}
		return nil
	}
	// Windows и macOS понимают в PAC HTTP-прокси надёжнее SOCKS5: подставляем HTTP-вход ядра
	pac := socksRe.ReplaceAllStringFunc(v.PacScript.Data, func(m string) string {
		port, _ := strconv.Atoi(socksRe.FindStringSubmatch(m)[1])
		if port == st.Port && st.HTTPPort > 0 {
			return "PROXY 127.0.0.1:" + strconv.Itoa(st.HTTPPort) + "; " + m
		}
		return "PROXY 127.0.0.1:" + strconv.Itoa(port) // заглушка kill switch: порт, где никого нет
	})
	a.mu.Lock()
	a.pac = pac
	a.pacSeq++
	seq := a.pacSeq
	a.proxyOn = true
	a.mu.Unlock()
	// новый адрес при каждом изменении: Windows кэширует PAC по адресу
	return sysproxy.SetPAC(a.local.base + "/proxy.pac?v=" + strconv.Itoa(seq))
}

func (a *App) currentPAC() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.pac
}

// Platform — как chrome.runtime.getPlatformInfo.
func (a *App) Platform() map[string]string {
	os := map[string]string{"windows": "win", "darwin": "mac", "linux": "linux"}[runtime.GOOS]
	arch := map[string]string{"amd64": "x86-64", "386": "x86-32", "arm64": "arm64"}[runtime.GOARCH]
	return map[string]string{"os": os, "arch": arch}
}

func (a *App) Version() string { return version }

// OpenURL открывает ссылку в браузере по умолчанию (только http и https).
func (a *App) OpenURL(u string) error {
	p, err := url.Parse(u)
	if err != nil || (p.Scheme != "https" && p.Scheme != "http") {
		return errors.New("недопустимая ссылка")
	}
	a.ui.openURL(u)
	return nil
}

// AuthPort — порт локального адреса, на который сайт вернёт итог входа.
func (a *App) AuthPort() int { return a.local.port }

// IsElevated — может ли ядро включить режим «Весь трафик» (права администратора есть
// у этого процесса или у службы).
func (a *App) IsElevated() bool { return a.svcc != nil || host.IsElevated() }

// RelaunchElevated перезапускает приложение с правами администратора (для режима TUN).
func (a *App) RelaunchElevated() error {
	if err := relaunchElevated(); err != nil {
		return err
	}
	a.shutdown()
	a.ui.quit()
	return nil
}

// Fetch — запрос к сайту аккаунта от имени интерфейса. Окно приложения не может
// обратиться к сайту напрямую (CORS), а расширению это разрешает браузер. Пускаем
// только на сайт аккаунта, GitHub (файл версий) и локальные адреса (проверка).
func (a *App) Fetch(rawURL, method, headersJSON string) (map[string]any, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	allowed := (u.Scheme == "https" && (u.Host == "tapp.internet-safe.online" || u.Host == "raw.githubusercontent.com")) ||
		(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost"))
	if !allowed {
		return nil, errors.New("адрес не разрешён: " + u.Host)
	}
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodPost {
		return nil, errors.New("метод не разрешён")
	}
	req, err := http.NewRequest(method, u.String(), nil)
	if err != nil {
		return nil, err
	}
	var hs map[string]string
	json.Unmarshal([]byte(headersJSON), &hs)
	for k, v := range hs {
		if k == "Authorization" || k == "Accept" {
			req.Header.Set(k, v)
		}
	}
	req.Header.Set("User-Agent", "SafeBridge-Desktop/"+version)
	client := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	return map[string]any{"status": resp.StatusCode, "body": string(body)}, nil
}

// ShowWindow показывает окно (из значка в строке меню или после перехода в настройки).
func (a *App) ShowWindow() { a.ui.show() }

// Quit завершает приложение (пункт «Выйти» в строке меню).
func (a *App) Quit() { a.ui.quit() }
