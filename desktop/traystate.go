package main

import (
	"encoding/json"
	"os"

	"github.com/safebridge/desktop/internal/tray"
)

// TraySet — значок в строке меню (macOS): интерфейс присылает состояние и пункты меню,
// выбор пункта возвращается событием "tray".
func (a *App) TraySet(stateJSON string) {
	if fakeTray {
		a.mu.Lock()
		a.trayLast = stateJSON
		a.mu.Unlock()
		return
	}
	if !tray.Available {
		return
	}
	var s struct {
		Visible bool        `json:"visible"`
		On      bool        `json:"on"`
		Tooltip string      `json:"tooltip"`
		Items   []tray.Item `json:"items"`
	}
	if json.Unmarshal([]byte(stateJSON), &s) != nil || !s.Visible {
		tray.Remove()
		return
	}
	tray.Set(s.On, s.Tooltip, s.Items)
}

// trayWanted — включён ли значок в строке меню (читаем при запуске: от этого зависит,
// прячется ли окно при закрытии или приложение завершается).
// fakeTray — проверка меню значка без macOS (тесты): меню запоминается, «нажатие» — TrayClick.
var fakeTray = os.Getenv("SAFEBRIDGE_FAKE_TRAY") != ""

func (a *App) TrayLast() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.trayLast
}

func (a *App) TrayClick(id string) {
	if fakeTray {
		a.emit("tray", id)
	}
}

func trayWanted() bool {
	if !tray.Available {
		return false
	}
	var st struct {
		AppTray *bool `json:"appTray"`
	}
	if b, err := os.ReadFile(settingsPath()); err == nil {
		json.Unmarshal(b, &st)
	}
	return st.AppTray == nil || *st.AppTray
}
