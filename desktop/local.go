package main

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"
)

// localServer — адрес на 127.0.0.1 для системного PAC и для возврата из входа через Google.
type localServer struct {
	port int
	base string
}

const authPage = `<!doctype html><html lang="ru"><meta charset="utf-8"><meta name="referrer" content="no-referrer">
<title>SafeBridge — вход</title>
<style>body{font:16px system-ui,sans-serif;max-width:440px;margin:18vh auto;text-align:center;color:#19212c;background:#f3f4f6}
h1{font-size:22px}@media(prefers-color-scheme:dark){body{background:#141b24;color:#e8edf3}}</style>
<h1 id="t">Возвращаемся в SafeBridge…</h1><p id="x"></p>
<script>
(function(){
  var h = location.hash.slice(1);
  history.replaceState(null, '', location.pathname); // токен не остаётся в истории
  var err = new URLSearchParams(h).get('error');
  var msgs = {not_linked: 'Этот Google-аккаунт не привязан к боту. Откройте бота, получите ключ и нажмите «Привязать Google».',
    cancelled: 'Вход отменён.', banned: 'Доступ к аккаунту ограничен.', google_not_configured: 'Вход через Google на сайте не настроен.'};
  fetch('/auth/complete', {method:'POST', headers:{'Content-Type':'application/json'},
    body: JSON.stringify(Object.fromEntries(new URLSearchParams(h)))})
  .then(function(r){ return r.ok; })
  .then(function(ok){
    if (err) { ok = false; }
    document.getElementById('t').textContent = ok ? 'Готово!' : 'Не получилось войти';
    document.getElementById('x').textContent = ok ? 'Вернитесь в приложение SafeBridge. Эту вкладку можно закрыть.'
      : (msgs[err] || 'Нажмите «Войти» в приложении ещё раз.');
  });
})();
</script></html>`

func startLocal(a *App) *localServer {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	s := &localServer{port: ln.Addr().(*net.TCPAddr).Port}
	s.base = "http://127.0.0.1:" + strconv.Itoa(s.port)
	mux := http.NewServeMux()
	mux.HandleFunc("/proxy.pac", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
		w.Header().Set("Cache-Control", "no-store")
		io.WriteString(w, a.currentPAC())
	})
	mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		io.WriteString(w, authPage)
	})
	mux.HandleFunc("/auth/complete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "", http.StatusMethodNotAllowed)
			return
		}
		var p map[string]string
		if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&p); err != nil {
			http.Error(w, "", http.StatusBadRequest)
			return
		}
		// токен проверит интерфейс: он примет его, только если nonce совпадёт с начатым входом
		b, _ := json.Marshal(p)
		a.emit("auth", string(b))
		if a.ui.show != nil {
			a.ui.show()
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln)
	return s
}
