//go:build embedcore

package host

import _ "embed"

// core/xray.gz кладёт скрипт сборки (build.sh) под нужную ОС/архитектуру.
//
//go:embed core/xray.gz
var embeddedCore []byte
