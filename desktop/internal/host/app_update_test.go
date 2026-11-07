//go:build !windows

package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func script(ver string) []byte { return []byte("#!/bin/sh\necho SafeBridge " + ver + "\n") }

func TestReplaceExe(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "SafeBridge.exe")
	os.WriteFile(exe, script("1.0.0"), 0o755)

	// новая версия не та (не запускается как надо) — ничего не меняем
	if _, err := replaceExe(exe, script("0.9.0"), "1.0.1"); err == nil {
		t.Fatal("ожидалась ошибка проверки версии")
	}
	if b, _ := os.ReadFile(exe); !strings.Contains(string(b), "1.0.0") {
		t.Fatal("старая версия испорчена")
	}

	p, err := replaceExe(exe, script("1.0.1"), "1.0.1")
	if err != nil || p != exe {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); !strings.Contains(string(b), "1.0.1") {
		t.Fatal("новая версия не встала")
	}
	if _, err := os.Stat(exe + ".old"); err != nil {
		t.Fatal("нет .old для отката")
	}
}

func TestAppPlatformKey(t *testing.T) {
	old := variant
	defer func() { variant = old }()
	variant = "win7"
	if k := appPlatformKey(); !strings.HasPrefix(k, "windows7-") {
		t.Fatal(k)
	}
}
