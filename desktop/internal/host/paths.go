package host

import (
	"os"
	"path/filepath"
	"runtime"
)

const (
	appName  = "SafeBridge"
	hostName = "com.vlessbridge.host"
)

// Задаются при сборке через -ldflags "-X main.version=... -X main.extensionIDs=id1,id2".
var (
	version      = "1.0.0" // версия приложения (совпадает с main.version)
	extensionIDs = "mpmmjdhamocialllnfmcpkjealmekhpl"
	// ID расширения в Firefox (browser_specific_settings.gecko.id)
	firefoxIDs = "vlessbridge@safebridge"
	// "win7" — сборка для Windows 7 (пропатченный Go от XTLS и ядро Xray-win7).
	// Такой помощник обновляется только на сборку для Windows 7 и берёт ядро Xray-win7-*.
	variant = ""
)

func isWin7Build() bool { return variant == "win7" }

func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return h
}

// DataDir — каталог приложения (отдельный от помощника расширения).
func DataDir() string { return dataDir() }

// dataDir — пользовательский каталог помощника (права администратора не нужны).
func dataDir() string {
	if d := os.Getenv("SAFEBRIDGE_DATA_DIR"); d != "" {
		return d
	}
	switch runtime.GOOS {
	case "windows":
		if d := os.Getenv("LOCALAPPDATA"); d != "" {
			return filepath.Join(d, appName)
		}
		return filepath.Join(homeDir(), "AppData", "Local", appName)
	case "darwin":
		return filepath.Join(homeDir(), "Library", "Application Support", appName)
	default:
		if d := os.Getenv("XDG_DATA_HOME"); d != "" {
			return filepath.Join(d, "safebridge")
		}
		return filepath.Join(homeDir(), ".local", "share", "safebridge")
	}
}
