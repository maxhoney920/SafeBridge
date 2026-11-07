// Package tray — значок SafeBridge в строке меню macOS (рядом с часами): состояние
// и быстрые настройки. На других системах значка нет (Available == false).
package tray

// Item — пункт меню. Пункт с Items — подменю; Sep — разделитель.
type Item struct {
	ID       string `json:"id,omitempty"`
	Title    string `json:"title,omitempty"`
	Checked  bool   `json:"checked,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
	Sep      bool   `json:"sep,omitempty"`
	Items    []Item `json:"items,omitempty"`
}

// OnClick вызывается при выборе пункта (с его ID), не в главном потоке.
var OnClick func(id string)
