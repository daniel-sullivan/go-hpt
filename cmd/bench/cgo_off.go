//go:build !(cgo && (linux || darwin))

package main

func cgoEnabled() bool { return false }
