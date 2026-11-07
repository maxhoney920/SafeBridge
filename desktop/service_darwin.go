//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	helperPath     = "/Library/PrivilegedHelperTools/" + serviceLabel
	plistPath      = "/Library/LaunchDaemons/" + serviceLabel + ".plist"
	serviceDataDir = "/Library/Application Support/SafeBridge/service"
)

func serviceInstalled() bool {
	_, err := os.Stat(plistPath)
	return err == nil
}

func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func xmlEsc(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// runAsAdmin выполняет скрипт от имени администратора: macOS показывает своё окно с паролем.
func runAsAdmin(script, prompt string) error {
	dir, err := os.MkdirTemp("", "safebridge-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	f := filepath.Join(dir, "run.sh")
	if err := os.WriteFile(f, []byte("#!/bin/sh\nset -e\n"+script+"\n"), 0o700); err != nil {
		return err
	}
	as := fmt.Sprintf(`do shell script %q with prompt %q with administrator privileges`, "/bin/sh "+shq(f), prompt)
	out, err := exec.Command("osascript", "-e", as).CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "-128") {
			return fmt.Errorf("отменено")
		}
		return fmt.Errorf("не получилось: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// installService копирует программу в системную папку (её не подменить без прав
// администратора) и регистрирует службу в launchd.
func installService() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	dir, err := os.MkdirTemp("", "safebridge-plist-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	tmpPlist := filepath.Join(dir, serviceLabel+".plist")
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>--daemon</string>
    <string>--uid</string><string>%d</string>
    <string>--socket</string><string>%s</string>
  </array>
  <key>EnvironmentVariables</key>
  <dict><key>SAFEBRIDGE_DATA_DIR</key><string>%s</string></dict>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>ProcessType</key><string>Interactive</string>
</dict>
</plist>
`, serviceLabel, xmlEsc(helperPath), os.Getuid(), xmlEsc(serviceSocket()), xmlEsc(serviceDataDir))
	if err := os.WriteFile(tmpPlist, []byte(plist), 0o644); err != nil {
		return err
	}
	script := strings.Join([]string{
		"launchctl bootout system/" + serviceLabel + " 2>/dev/null || launchctl unload " + shq(plistPath) + " 2>/dev/null || true",
		"sleep 1",
		"mkdir -p /Library/PrivilegedHelperTools",
		"install -m 755 -o root -g wheel " + shq(exe) + " " + shq(helperPath),
		"install -m 644 -o root -g wheel " + shq(tmpPlist) + " " + shq(plistPath),
		"launchctl bootstrap system " + shq(plistPath) + " 2>/dev/null || launchctl load -w " + shq(plistPath),
	}, "\n")
	return runAsAdmin(script, "SafeBridge устанавливает службу, чтобы весь трафик компьютера шёл через VPN.")
}

func uninstallService() error {
	script := strings.Join([]string{
		"launchctl bootout system/" + serviceLabel + " 2>/dev/null || launchctl unload " + shq(plistPath) + " 2>/dev/null || true",
		"rm -f " + shq(plistPath) + " " + shq(helperPath) + " " + shq(serviceSocket()),
		"rm -rf " + shq(filepath.Dir(serviceDataDir)),
	}, "\n")
	return runAsAdmin(script, "SafeBridge удаляет службу.")
}
