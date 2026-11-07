//go:build windows

package notify

import (
	"encoding/base64"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unicode/utf16"
)

// Уведомление в области уведомлений через PowerShell и NotifyIcon: работает на Windows
// 7–11 без сторонних библиотек (в Windows 10/11 оно показывается как обычное уведомление).
// Текст передаётся через -EncodedCommand, чтобы кириллица дошла без искажений.
func show(title, text, link string) error {
	exe, _ := os.Executable()
	q := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	click := "$global:done = $true"
	if link != "" {
		click = "Start-Process " + q(link) + "; $global:done = $true"
	}
	script := strings.Join([]string{
		"Add-Type -AssemblyName System.Windows.Forms, System.Drawing",
		"$n = New-Object System.Windows.Forms.NotifyIcon",
		"try { $n.Icon = [System.Drawing.Icon]::ExtractAssociatedIcon(" + q(exe) + ") } catch { $n.Icon = [System.Drawing.SystemIcons]::Information }",
		"$n.Text = 'SafeBridge'",
		"$n.Visible = $true",
		"$global:done = $false",
		"$n.add_BalloonTipClicked({ " + click + " })",
		"$n.add_Click({ " + click + " })",
		"$n.add_BalloonTipClosed({ $global:done = $true })",
		"$n.ShowBalloonTip(20000, " + q(title) + ", " + q(text) + ", [System.Windows.Forms.ToolTipIcon]::Warning)",
		"$t = [DateTime]::Now.AddSeconds(40)",
		"while (-not $global:done -and [DateTime]::Now -lt $t) { [System.Windows.Forms.Application]::DoEvents(); Start-Sleep -Milliseconds 150 }",
		"$n.Dispose()",
	}, "\n")
	u := utf16.Encode([]rune(script))
	b := make([]byte, len(u)*2)
	for i, c := range u {
		b[2*i], b[2*i+1] = byte(c), byte(c>>8)
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-EncodedCommand", base64.StdEncoding.EncodeToString(b))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	return cmd.Start()                                                                  // не ждём: PowerShell сам закроется после уведомления
}
