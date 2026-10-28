package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type browserTarget struct {
	Name     string
	Dir      string // каталог NativeMessagingHosts (macOS/Linux)
	Required bool   // создать, даже если браузер не найден
}

// browserTargets — каталоги манифестов Chromium-браузеров для macOS/Linux.
func browserTargets() []browserTarget {
	h := homeDir()
	if runtime.GOOS == "darwin" {
		base := filepath.Join(h, "Library", "Application Support")
		return []browserTarget{
			{"Google Chrome", filepath.Join(base, "Google", "Chrome"), true},
			{"Chrome Beta", filepath.Join(base, "Google", "Chrome Beta"), false},
			{"Chromium", filepath.Join(base, "Chromium"), false},
			{"Microsoft Edge", filepath.Join(base, "Microsoft Edge"), false},
			{"Brave", filepath.Join(base, "BraveSoftware", "Brave-Browser"), false},
			{"Vivaldi", filepath.Join(base, "Vivaldi"), false},
			{"Яндекс Браузер", filepath.Join(base, "Yandex", "YandexBrowser"), false},
			{"Arc", filepath.Join(base, "Arc", "User Data"), false},
		}
	}
	cfg := os.Getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		cfg = filepath.Join(h, ".config")
	}
	return []browserTarget{
		{"Google Chrome", filepath.Join(cfg, "google-chrome"), true},
		{"Chrome Beta", filepath.Join(cfg, "google-chrome-beta"), false},
		{"Chromium", filepath.Join(cfg, "chromium"), false},
		{"Microsoft Edge", filepath.Join(cfg, "microsoft-edge"), false},
		{"Brave", filepath.Join(cfg, "BraveSoftware", "Brave-Browser"), false},
		{"Vivaldi", filepath.Join(cfg, "vivaldi"), false},
		{"Яндекс Браузер", filepath.Join(cfg, "yandex-browser"), false},
	}
}

func allowedOrigins(extra []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, id := range append(strings.Split(extensionIDs, ","), extra...) {
		id = strings.TrimSpace(id)
		if len(id) == 32 && !seen[id] {
			seen[id] = true
			out = append(out, "chrome-extension://"+id+"/")
		}
	}
	return out
}

func manifestJSON(exePath string, extra []string) []byte {
	m := map[string]any{
		"name":            hostName,
		"description":     "VlessBridge native helper (sing-box)",
		"path":            exePath,
		"type":            "stdio",
		"allowed_origins": allowedOrigins(extra),
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	return b
}

func copySelf(dst string) error {
	src, err := os.Executable()
	if err != nil {
		return err
	}
	if s, err := filepath.EvalSymlinks(src); err == nil {
		src = s
	}
	if d, err := filepath.EvalSymlinks(dst); err == nil && strings.EqualFold(d, src) {
		return nil // уже запущены из места установки
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	// работающий exe на Windows нельзя перезаписать, но можно переименовать
	if _, err := os.Stat(dst); err == nil {
		old := dst + ".old"
		os.Remove(old)
		if err := os.Rename(dst, old); err != nil {
			return fmt.Errorf("не удалось заменить %s (закройте браузер и повторите): %v", dst, err)
		}
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func runInstall(extraIDs []string) error {
	dir := dataDir()
	exe := filepath.Join(dir, exeName("vlessbridge"))
	fmt.Printf("VlessBridge %s — установка помощника\n\n", version)
	fmt.Println("• Каталог:", dir)
	if err := copySelf(exe); err != nil {
		return err
	}
	// распаковываем ядро сразу, чтобы первый запуск из браузера был быстрым
	os.Setenv("VLESSBRIDGE_DATA_DIR", dir)
	if p, err := CorePath(); err != nil {
		fmt.Println("• Ядро: ВНИМАНИЕ —", err)
	} else {
		v := CoreVersion()
		if v == "" {
			v = "?"
		}
		fmt.Printf("• Ядро sing-box %s: %s\n", v, p)
	}
	man := manifestJSON(exe, extraIDs)
	if runtime.GOOS == "windows" {
		mp := filepath.Join(dir, hostName+".json")
		if err := os.WriteFile(mp, man, 0o644); err != nil {
			return err
		}
		registered, err := registerWindows(mp)
		if err != nil {
			return err
		}
		fmt.Println("• Зарегистрирован для:", strings.Join(registered, ", "))
	} else {
		var done []string
		for _, b := range browserTargets() {
			if _, err := os.Stat(b.Dir); err != nil && !b.Required {
				continue
			}
			nm := filepath.Join(b.Dir, "NativeMessagingHosts")
			if err := os.MkdirAll(nm, 0o755); err != nil {
				fmt.Printf("  ! %s: %v\n", b.Name, err)
				continue
			}
			if err := os.WriteFile(filepath.Join(nm, hostName+".json"), man, 0o644); err != nil {
				fmt.Printf("  ! %s: %v\n", b.Name, err)
				continue
			}
			done = append(done, b.Name)
		}
		fmt.Println("• Зарегистрирован для:", strings.Join(done, ", "))
	}
	fmt.Println("\nГотово! Вернитесь в браузер и нажмите «Проверить снова» в окне расширения.")
	return nil
}

func runUninstall() error {
	dir := dataDir()
	if runtime.GOOS == "windows" {
		unregisterWindows()
	} else {
		for _, b := range browserTargets() {
			os.Remove(filepath.Join(b.Dir, "NativeMessagingHosts", hostName+".json"))
		}
	}
	// сам exe на Windows удалить нельзя, пока он запущен — удаляем всё, что получится
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		os.RemoveAll(filepath.Join(dir, e.Name()))
	}
	os.Remove(dir)
	fmt.Println("Помощник VlessBridge удалён. Расширение удалите в настройках браузера.")
	return nil
}
