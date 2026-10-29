package main

import (
	"os"
	"path/filepath"
	"runtime"
)

const (
	appName  = "VlessBridge"
	hostName = "com.vlessbridge.host"
)

// Задаются при сборке через -ldflags "-X main.version=... -X main.extensionIDs=id1,id2".
var (
	version      = "dev"
	extensionIDs = "mpmmjdhamocialllnfmcpkjealmekhpl"
	// ID расширения в Firefox (browser_specific_settings.gecko.id)
	firefoxIDs = "vlessbridge@safebridge"
)

func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return h
}

// dataDir — пользовательский каталог помощника (права администратора не нужны).
func dataDir() string {
	if d := os.Getenv("VLESSBRIDGE_DATA_DIR"); d != "" {
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
			return filepath.Join(d, "vlessbridge")
		}
		return filepath.Join(homeDir(), ".local", "share", "vlessbridge")
	}
}
