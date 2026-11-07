//go:build !embedcore

package host

// Сборка без встроенного ядра: xray ищется рядом с программой или в PATH.
var embeddedCore []byte
