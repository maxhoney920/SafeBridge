//go:build windows

package main

// На Windows служба не нужна: приложение перезапускается с правами администратора.
func runDaemonIfRequested() bool { return false }
