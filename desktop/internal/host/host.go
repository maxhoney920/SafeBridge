package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"time"
)

// Протокол native messaging: 4 байта длины (порядок байт платформы, на всех целевых — LE) + JSON.

type request struct {
	ID   int             `json:"id"`
	Cmd  string          `json:"cmd"`
	Args json.RawMessage `json:"args"`
}

type response struct {
	ID     int    `json:"id"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
	Result any    `json:"result,omitempty"`
}

type event struct {
	Event string `json:"event"`
	Data  any    `json:"data,omitempty"`
}

// Host — то же, что помощник расширения, но внутри приложения: команды приходят не
// через native messaging, а вызовом HandleJSON, ответы и события уходят в emit.
type Host struct {
	emit func([]byte)
	core *Core
}

// New создаёт ядро приложения. emit получает JSON ответов {id, ok, result|error}
// и событий {event, data} — тех же, что помощник отправлял расширению.
func New(emit func([]byte)) *Host {
	dir := dataDir()
	os.MkdirAll(dir, 0o700)
	if f, err := os.OpenFile(filepath.Join(dir, "app.log"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600); err == nil {
		log.SetOutput(f)
	}
	log.Printf("SafeBridge app %s started, pid %d", version, os.Getpid())
	h := &Host{emit: emit}
	h.core = NewCore(func(msg string) {
		log.Println(msg)
		h.send(event{Event: "core_exit", Data: map[string]any{"message": msg, "log": h.core.LogTail(20)}})
	})
	return h
}

// HandleJSON выполняет команду асинхронно, как помощник.
func (h *Host) HandleJSON(b []byte) {
	var req request
	if err := json.Unmarshal(b, &req); err != nil {
		h.send(response{Error: "некорректный JSON"})
		return
	}
	go h.handle(req)
}

// Shutdown останавливает ядро (при выходе из приложения).
func (h *Host) Shutdown() { h.core.Stop() }

// Status — состояние ядра (для системного прокси и PAC).
func (h *Host) Status() CoreStatus { return h.core.Status() }

func (h *Host) send(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		log.Println("marshal:", err)
		return
	}
	h.emit(b)
}

func (h *Host) handle(req request) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("panic in %s: %v", req.Cmd, r)
			h.send(response{ID: req.ID, Error: fmt.Sprint("внутренняя ошибка: ", r)})
		}
	}()
	res, err := h.dispatch(req)
	if err != nil {
		log.Printf("%s: %v", req.Cmd, redactURLs(err.Error()))
		h.send(response{ID: req.ID, Error: err.Error()})
		return
	}
	h.send(response{ID: req.ID, OK: true, Result: res})
}

// ссылки подписок содержат личный токен — в журнал на диске они не пишутся
var urlRe = regexp.MustCompile(`https?://[^\s"']+`)

func redactURLs(s string) string { return urlRe.ReplaceAllString(s, "<ссылка>") }

func (h *Host) proxyPort() int {
	if st := h.core.Status(); st.Running {
		return st.Port
	}
	return 0
}

func (h *Host) dispatch(req request) (any, error) {
	switch req.Cmd {
	case "hello":
		_, coreErr := CorePath()
		r := map[string]any{"version": version, "core": CoreVersion(), "bundledCore": bundledXray, "os": runtime.GOOS, "arch": runtime.GOARCH,
			"variant": variant, "status": h.core.Status()}
		if coreErr != nil {
			r["coreError"] = coreErr.Error()
		}
		return r, nil

	case "fetch":
		var a struct {
			URL string `json:"url"`
			UA  string `json:"ua"`
		}
		if err := json.Unmarshal(req.Args, &a); err != nil {
			return nil, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		st := h.core.Status()
		port := 0
		if st.Running {
			port = st.Port
		}
		return FetchSubscription(ctx, a.URL, a.UA, port)

	case "apply":
		var a struct {
			Servers       []ApplyServer `json:"servers"`
			Selected      string        `json:"selected"`
			PreferredPort int           `json:"preferredPort"`
			Tun           *TunOptions   `json:"tun"` // приложение: режим «весь трафик»
		}
		if err := json.Unmarshal(req.Args, &a); err != nil {
			return nil, err
		}
		return h.core.Apply(a.Servers, a.Selected, a.PreferredPort, a.Tun)

	case "select":
		var a struct {
			ID string `json:"id"`
		}
		json.Unmarshal(req.Args, &a)
		if err := h.core.Select(a.ID); err != nil {
			return nil, err
		}
		return h.core.Status(), nil

	case "ping":
		var a struct {
			IDs       []string `json:"ids"`
			URL       string   `json:"url"`
			TimeoutMs int      `json:"timeoutMs"`
		}
		json.Unmarshal(req.Args, &a)
		if a.URL == "" {
			a.URL = "https://www.gstatic.com/generate_204"
		}
		if a.TimeoutMs <= 0 {
			a.TimeoutMs = 5000
		}
		if !h.core.Status().Running {
			return nil, errors.New("ядро не запущено")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		return h.core.DelayMany(ctx, a.IDs, a.URL, a.TimeoutMs), nil

	case "checkUpdates":
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		info := CheckUpdates(ctx, h.proxyPort())
		// у приложения нет отдельного помощника: проверяем только ядро Xray
		info.Helper.Available, info.Helper.Latest = false, ""
		if isMacLegacy() {
			// новые Xray не запускаются на старых macOS: ядро обновляется вместе с приложением
			info.Core.Available, info.Core.Latest = false, ""
			info.Core.Error = "в версии для старых macOS ядро обновляется вместе с приложением"
		}
		return info, nil

	case "updateCore":
		if isMacLegacy() {
			return nil, errors.New("в версии для старых macOS ядро обновляется вместе с приложением: новые Xray на них не запускаются")
		}
		var a struct {
			Version string `json:"version"`
		}
		json.Unmarshal(req.Args, &a)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		return h.core.UpdateCore(ctx, a.Version, h.proxyPort())

	case "updateHelper", "uninstall":
		// в приложении помощника нет: обновляется само приложение
		return nil, errors.New("в приложении эта команда не нужна")

	case "stop":
		h.core.Stop()
		return h.core.Status(), nil

	case "status":
		return map[string]any{"status": h.core.Status(), "log": h.core.LogTail(50), "dataDir": dataDir()}, nil

	case "parse":
		// разбор подписки, вставленной текстом (без скачивания)
		var a struct {
			Text string `json:"text"`
		}
		json.Unmarshal(req.Args, &a)
		return ParseSubscription([]byte(a.Text))
	}
	return nil, fmt.Errorf("неизвестная команда %q", req.Cmd)
}
