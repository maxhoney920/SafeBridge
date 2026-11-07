//go:build !darwin || !cgo

package tray

const Available = false

func Set(on bool, tooltip string, items []Item) {}

func Remove() {}
