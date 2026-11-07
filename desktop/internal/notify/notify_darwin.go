//go:build darwin && cgo

package notify

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>
void sbNotify(const char *title, const char *text, const char *link);
*/
import "C"

import "unsafe"

func show(title, text, link string) error {
	ct, cx, cl := C.CString(title), C.CString(text), C.CString(link)
	defer C.free(unsafe.Pointer(ct))
	defer C.free(unsafe.Pointer(cx))
	defer C.free(unsafe.Pointer(cl))
	C.sbNotify(ct, cx, cl)
	return nil
}
