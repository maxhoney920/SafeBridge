//go:build darwin && cgo

package tray

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>
void sbTraySet(const void *png, int pngLen, const char *menuJSON, const char *tooltip);
void sbTrayRemove(void);
const char *sbFixWindowButtons(void);
*/
import "C"

import (
	_ "embed"
	"encoding/json"
	"unsafe"
)

const Available = true

//go:embed icons/on.png
var iconOn []byte

//go:embed icons/off.png
var iconOff []byte

// Set показывает значок (или обновляет его): on — VPN подключён.
func Set(on bool, tooltip string, items []Item) {
	b, _ := json.Marshal(items)
	icon := iconOff
	if on {
		icon = iconOn
	}
	cm := C.CString(string(b))
	ct := C.CString(tooltip)
	defer C.free(unsafe.Pointer(cm))
	defer C.free(unsafe.Pointer(ct))
	// Objective-C копирует данные сразу, а меню собирает в главном потоке
	C.sbTraySet(unsafe.Pointer(&icon[0]), C.int(len(icon)), cm, ct)
}

// Remove убирает значок из строки меню.
func Remove() { C.sbTrayRemove() }

//export sbTrayClick
func sbTrayClick(id *C.char) {
	s := C.GoString(id)
	if OnClick != nil {
		go OnClick(s)
	}
}

// FixWindowButtons показывает кнопки окна, если система их скрыла (macOS 10.13), и
// возвращает их состояние до исправления — для журнала.
func FixWindowButtons() string {
	c := C.sbFixWindowButtons()
	defer C.free(unsafe.Pointer(c))
	return C.GoString(c)
}
