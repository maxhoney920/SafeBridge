//go:build !windows

package main

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func hideWindow(cmd *exec.Cmd) {}

func prepareChild(cmd *exec.Cmd) {
	cmd.SysProcAttr = childAttrs()
}

func afterStart(cmd *exec.Cmd) {}

func terminate(cmd *exec.Cmd) {
	if cmd.Process != nil {
		cmd.Process.Signal(syscall.SIGTERM)
	}
}

// killStaleCore убивает ядро, оставшееся от аварийно завершённого помощника,
// только если PID действительно принадлежит ядру.
func killStaleCore(pidFile string) {
	b, err := os.ReadFile(pidFile)
	if err != nil {
		return
	}
	os.Remove(pidFile)
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 1 {
		return
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil || !strings.Contains(string(out), coreName) {
		return
	}
	syscall.Kill(pid, syscall.SIGTERM)
}

// scheduleDelete на macOS/Linux не нужен: запущенный файл можно удалить сразу.
func scheduleDelete(dir string) error { return os.RemoveAll(dir) }
