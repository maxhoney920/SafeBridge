//go:build !windows

package main

func registerWindows(string) ([]string, error) { return nil, nil }
func unregisterWindows()                       {}
