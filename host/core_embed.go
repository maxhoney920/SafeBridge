//go:build embedcore

package main

import _ "embed"

// core/sing-box.gz кладёт скрипт сборки (build.sh) под нужную ОС/архитектуру.
//
//go:embed core/sing-box.gz
var embeddedCore []byte
