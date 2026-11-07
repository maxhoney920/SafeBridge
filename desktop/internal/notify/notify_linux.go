//go:build linux

package notify

import "os/exec"

func show(title, text, link string) error {
	args := []string{"--app-name=SafeBridge", title, text}
	if link != "" {
		args[2] = text + "\n" + link
	}
	return exec.Command("notify-send", args...).Run()
}
