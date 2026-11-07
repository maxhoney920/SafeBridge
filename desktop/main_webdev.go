//go:build webdev

// Режим разработки и проверки: тот же интерфейс в обычном браузере, мост — по HTTP.
// go run -tags webdev . → http://127.0.0.1:28200/
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"sync"
	"syscall"
)

func main() {
	if handleCLI() || runDaemonIfRequested() {
		return
	}
	app := NewApp()
	var mu sync.Mutex
	subs := map[chan string]bool{}
	app.emit = func(name, data string) {
		b, _ := json.Marshal(map[string]string{"name": name, "data": data})
		mu.Lock()
		for c := range subs {
			select {
			case c <- string(b):
			default:
			}
		}
		mu.Unlock()
	}
	opened := []string{}
	app.ui = uiHooks{
		openURL: func(u string) { mu.Lock(); opened = append(opened, u); mu.Unlock(); log.Println("open:", u) },
		show:    func() {},
		quit:    func() { os.Exit(0) },
	}
	addr := "127.0.0.1:28200"
	if v := os.Getenv("SB_ADDR"); v != "" {
		addr = v
	}
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir("frontend")))
	mux.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		c := make(chan string, 256)
		mu.Lock()
		subs[c] = true
		mu.Unlock()
		defer func() { mu.Lock(); delete(subs, c); mu.Unlock() }()
		fl := w.(http.Flusher)
		fmt.Fprint(w, ": ok\n\n")
		fl.Flush()
		for {
			select {
			case m := <-c:
				fmt.Fprintf(w, "data: %s\n\n", m)
				fl.Flush()
			case <-r.Context().Done():
				return
			}
		}
	})
	mux.HandleFunc("/api/opened", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		json.NewEncoder(w).Encode(opened)
	})
	// /api/call {method, args:[...]} → результат метода App (как привязки Wails)
	mux.HandleFunc("/api/call", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string            `json:"method"`
			Args   []json.RawMessage `json:"args"`
		}
		b, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(b, &req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		m := reflect.ValueOf(app).MethodByName(req.Method)
		if !m.IsValid() || req.Method == "" || req.Method[0] < 'A' || req.Method[0] > 'Z' {
			http.Error(w, "no method", 404)
			return
		}
		in := []reflect.Value{}
		for i := 0; i < m.Type().NumIn(); i++ {
			v := reflect.New(m.Type().In(i))
			if i < len(req.Args) {
				json.Unmarshal(req.Args[i], v.Interface())
			}
			in = append(in, v.Elem())
		}
		out := m.Call(in)
		res := map[string]any{}
		for _, o := range out {
			if e, ok := o.Interface().(error); ok && e != nil {
				res["error"] = e.Error()
			} else if o.Type().Name() != "error" {
				res["result"] = o.Interface()
			}
		}
		json.NewEncoder(w).Encode(res)
	})
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() { <-sig; app.shutdown(); os.Exit(0) }()
	log.Println("SafeBridge (режим разработки):", "http://"+addr+"/")
	log.Fatal(http.ListenAndServe(addr, mux))
}
