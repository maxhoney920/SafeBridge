//go:build !embedcore

package main

// Сборка без встроенного ядра: sing-box ищется рядом с программой или в PATH.
var embeddedCore []byte
