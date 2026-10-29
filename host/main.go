// VlessBridge — помощник (native messaging host) для браузерного расширения.
//
// Запуск без аргументов (двойной клик) — установка для текущего пользователя.
// Браузер запускает его в режиме помощника: Chrome — с аргументом chrome-extension://<id>/,
// Firefox — с путём к манифесту и ID расширения.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	args := os.Args[1:]
	if len(args) > 0 && (strings.HasPrefix(args[0], "chrome-extension://") ||
		(strings.HasSuffix(strings.ToLower(args[0]), ".json") && len(args) >= 2)) {
		runHost()
		return
	}
	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
		args = args[1:]
	}
	switch cmd {
	case "", "install":
		fs := flag.NewFlagSet("install", flag.ExitOnError)
		ids := fs.String("extension-id", "", "дополнительные ID расширения через запятую")
		fs.Parse(args)
		var extra []string
		if *ids != "" {
			extra = strings.Split(*ids, ",")
		}
		err := runInstall(extra)
		if err != nil {
			fmt.Println("\nОШИБКА:", err)
		}
		if cmd == "" {
			pause()
		}
		if err != nil {
			os.Exit(1)
		}
	case "uninstall":
		runUninstall()
	case "version":
		fmt.Printf("vlessbridge %s, Xray %s\n", version, CoreVersion())
	case "parse":
		// отладка: vlessbridge parse <файл подписки>
		if len(args) < 1 {
			fmt.Println("usage: vlessbridge parse <file>")
			os.Exit(2)
		}
		b, err := os.ReadFile(args[0])
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		res, err := ParseSubscription(b)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		out, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(out))
	default:
		fmt.Println("Использование: vlessbridge [install [--extension-id ID] | uninstall | version | parse <file>]")
		os.Exit(2)
	}
}

func pause() {
	fmt.Print("\nНажмите Enter, чтобы закрыть окно…")
	bufio.NewReader(os.Stdin).ReadString('\n')
}
