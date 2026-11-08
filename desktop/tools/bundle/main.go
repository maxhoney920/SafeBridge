// Сборка интерфейса приложения в обычные скрипты (без ES-модулей) с синтаксисом для
// Safari 13: старый WebKit (macOS 10.13–11) не загружает модули со схемы wails:// и не
// знает часть нового синтаксиса. Запускается из sync-frontend.sh: go run ./tools/bundle
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

func main() {
	dir := "frontend"
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
			Engines:       []api.Engine{{Name: api.EngineSafari, Version: "13"}, {Name: api.EngineChrome, Version: "87"}},
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
		s := strings.Replace(string(b), old, `<script src="`+out+`"></script>`, 1)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			panic(err)
		}
	}
}
