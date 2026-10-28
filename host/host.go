package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
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

type Host struct {
	out  io.Writer
	wmu  sync.Mutex
	core *Core
}

func (h *Host) send(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		log.Println("marshal:", err)
		return
	}
	if len(b) > 1<<20 {
		b, _ = json.Marshal(response{ID: idOf(v), Error: "ответ больше 1 МБ"})
	}
	h.wmu.Lock()
	defer h.wmu.Unlock()
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], uint32(len(b)))
	h.out.Write(hdr[:])
	h.out.Write(b)
}

func idOf(v any) int {
	if r, ok := v.(response); ok {
		return r.ID
	}
	return 0
}

func readMsg(r *bufio.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.LittleEndian.Uint32(hdr[:])
	if n > 64<<20 {
		return nil, fmt.Errorf("слишком большое сообщение: %d", n)
	}
	b := make([]byte, n)
	_, err := io.ReadFull(r, b)
	return b, err
}

func runHost() {
	dir := dataDir()
	os.MkdirAll(dir, 0o700)
	if f, err := os.OpenFile(filepath.Join(dir, "host.log"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600); err == nil {
		log.SetOutput(f)
		defer f.Close()
	}
	log.Printf("host %s started, pid %d", version, os.Getpid())
	cleanupOldHelper()

	h := &Host{out: os.Stdout}
	h.core = NewCore(func(msg string) {
		log.Println(msg)
		h.send(event{Event: "core_exit", Data: map[string]any{"message": msg, "log": h.core.LogTail(20)}})
	})
	defer h.core.Stop()

	in := bufio.NewReader(os.Stdin)
	for {
		b, err := readMsg(in)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Println("read:", err)
			}
			log.Println("браузер закрыл соединение — останавливаю ядро")
			return
		}
		var req request
		if err := json.Unmarshal(b, &req); err != nil {
			h.send(response{Error: "некорректный JSON"})
			continue
		}
		go h.handle(req)
	}
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
		log.Printf("%s: %v", req.Cmd, err)
		h.send(response{ID: req.ID, Error: err.Error()})
		return
	}
	h.send(response{ID: req.ID, OK: true, Result: res})
}

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
			"status": h.core.Status()}
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
		}
		if err := json.Unmarshal(req.Args, &a); err != nil {
			return nil, err
		}
		return h.core.Apply(a.Servers, a.Selected, a.PreferredPort)

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
		return CheckUpdates(ctx, h.proxyPort()), nil

	case "updateCore":
		var a struct {
			Version string `json:"version"`
		}
		json.Unmarshal(req.Args, &a)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		return h.core.UpdateCore(ctx, a.Version, h.proxyPort())

	case "updateHelper":
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		res, err := UpdateHelper(ctx, h.proxyPort())
		if err == nil && res["updated"] == true {
			// ответить и завершиться: браузер запустит новую версию при переподключении
			go func() {
				time.Sleep(500 * time.Millisecond)
				h.core.Stop()
				log.Println("помощник обновлён, перезапуск")
				os.Exit(0)
			}()
		}
		return res, err

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
