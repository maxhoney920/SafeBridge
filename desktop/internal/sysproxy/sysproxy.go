// Package sysproxy включает в системе автонастройку прокси (PAC) и возвращает прежние
// настройки при выходе. Прежние значения сохраняются в файл: если приложение упадёт,
// при следующем запуске Recover вернёт их, и интернет не останется «сломанным».
package sysproxy

import (
	"encoding/json"
	"os"
	"path/filepath"
)

var backupPath string

// Init задаёт каталог для файла с прежними настройками.
func Init(dataDir string) { backupPath = filepath.Join(dataDir, "sysproxy-backup.json") }

func saveBackup(v any) {
	if backupPath == "" {
		return
	}
	if _, err := os.Stat(backupPath); err == nil {
		return // уже сохранены настройки до первого включения — их и возвращаем
	}
	b, _ := json.Marshal(v)
	os.WriteFile(backupPath, b, 0o600)
}

func loadBackup(v any) bool {
	b, err := os.ReadFile(backupPath)
	if err != nil {
		return false
	}
	return json.Unmarshal(b, v) == nil
}

func dropBackup() { os.Remove(backupPath) }

// Recover возвращает прежние настройки, если прошлый запуск не успел это сделать.
func Recover() error {
	if _, err := os.Stat(backupPath); err != nil {
		return nil
	}
	return Clear()
}
