//go:build !windows

package main

import (
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/safebridge/desktop/internal/host"
	"github.com/safebridge/desktop/internal/svc"
)

// runDaemonIfRequested — запуск в роли службы (SafeBridge --daemon): ядро с правами
// администратора без окна. На macOS так его запускает launchd после установки службы.
func runDaemonIfRequested() bool {
	if len(os.Args) < 2 || os.Args[1] != "--daemon" {
		return false
	}
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	uid := fs.Int("uid", -1, "пользователь, которому разрешено подключаться")
	sock := fs.String("socket", serviceSocket(), "путь к сокету")
	fs.Parse(os.Args[2:])
	if *sock == "" {
		log.Fatal("не указан сокет")
	}

	srv := &svc.Server{Version: version}
	h := host.New(srv.Emit)
	srv.Core = h
	srv.Allow = func(c net.Conn) bool {
		u, ok := svc.PeerUID(c)
		return !ok || *uid < 0 || u == 0 || u == *uid
	}

	syscall.Umask(0o077) // сокет создаётся сразу закрытым для чужих
	os.Remove(*sock)
	l, err := net.Listen("unix", *sock)
	if err != nil {
		log.Fatal(err)
	}
	os.Chmod(*sock, 0o600)
	if *uid >= 0 {
		os.Chown(*sock, *uid, -1)
	}
	log.Printf("служба SafeBridge %s: %s, пользователь %d", version, *sock, *uid)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sig
		h.Shutdown()
		os.Remove(*sock)
		os.Exit(0)
	}()
	if err := srv.Serve(l); err != nil {
		log.Fatal(err)
	}
	return true
}
