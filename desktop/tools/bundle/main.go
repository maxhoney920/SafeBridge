// Сборка интерфейса приложения в обычные скрипты (без ES-модулей) с синтаксисом для
// Safari 13: старый WebKit (macOS 10.13–11) не загружает модули со схемы wails:// и не
// знает часть нового синтаксиса. Запускается из sync-frontend.sh: go run ./tools/bundle
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

// Safari 11 — High Sierra без обновлений Safari; Chrome 87 — WebView2 на Windows 7 и новее.
var engines = []api.Engine{{Name: api.EngineSafari, Version: "11"}, {Name: api.EngineChrome, Version: "87"}}

func main() {
	dir := "frontend"
	wailsRuntime(dir)
	entries := map[string]string{ // страница → точка входа
		"index.html":   "src/boot.js",
		"popup.html":   "src/popup.js",
		"options.html": "src/options.js",
	}
	for page, entry := range entries {
		out := strings.TrimSuffix(entry, ".js") + ".bundle.js"
		r := api.Build(api.BuildOptions{
			EntryPoints:   []string{filepath.Join(dir, entry)},
			Outfile:       filepath.Join(dir, out),
			Bundle:        true,
			Write:         true,
			Format:        api.FormatIIFE,
			Engines:       engines,
			Charset:       api.CharsetUTF8,
			LegalComments: api.LegalCommentsNone,
			LogLevel:      api.LogLevelWarning,
		})
		if len(r.Errors) > 0 {
			for _, e := range r.Errors {
				fmt.Fprintln(os.Stderr, "bundle:", entry, e.Text, e.Location)
			}
			os.Exit(1)
		}
		p := filepath.Join(dir, page)
		b, err := os.ReadFile(p)
		if err != nil {
			panic(err)
		}
		old := `<script type="module" src="` + entry + `"></script>`
		if !strings.Contains(string(b), old) {
			fmt.Fprintln(os.Stderr, "bundle: в", page, "нет", old)
			os.Exit(1)
		}
		// скрипты встраиваем в страницу: со схемы wails:// старый WebKit считает внешние
		// скрипты «чужими» и вместо текста ошибки показывает только «Script error.»
		js, err := os.ReadFile(filepath.Join(dir, out))
		if err != nil {
			panic(err)
		}
		poly, err := os.ReadFile(filepath.Join(dir, "src/polyfills.js"))
		if err != nil {
			panic(err)
		}
		inline := func(code []byte) string {
			return "<script>\n" + strings.ReplaceAll(string(code), "</script", "<\\/script") + "\n</script>"
		}
		s := strings.Replace(string(b), old, inline(js), 1)
		s = strings.Replace(s, `<script src="src/polyfills.js"></script>`, inline(poly), 1)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			panic(err)
		}
	}
}

// wailsRuntime — копия встроенного скрипта Wails с синтаксисом для старого Safari: свой
// он пишет с «?.», которого Safari до 13.1 не знает, и тогда окно не связывается с
// приложением. bridge.js подключает эту копию, если встроенный скрипт не сработал.
func wailsRuntime(dir string) {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/wailsapp/wails/v2").Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "bundle: не найден модуль Wails:", err)
		os.Exit(1)
	}
	src := filepath.Join(strings.TrimSpace(string(out)), "internal", "frontend", "runtime", "runtime_prod_desktop.js")
	code, err := os.ReadFile(src)
	if err != nil {
		panic(err)
	}
	r := api.Transform(string(code), api.TransformOptions{Engines: engines, Charset: api.CharsetUTF8, LegalComments: api.LegalCommentsNone})
	if len(r.Errors) > 0 {
		fmt.Fprintln(os.Stderr, "bundle: runtime Wails:", r.Errors[0].Text)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "wails-runtime.js"), r.Code, 0o644); err != nil {
		panic(err)
	}
}
