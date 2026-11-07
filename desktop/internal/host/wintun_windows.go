//go:build windows && embedcore

package host

import _ "embed"

// wintun.dll из официального архива Xray для Windows (кладёт скрипт сборки)
//
//go:embed core/wintun.dll
var embeddedWintun []byte
