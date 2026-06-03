//go:build !wails

package main

import "fmt"

// This binary is the GUI entrypoint, but the GUI is only compiled with the
// "wails" build tag (which pulls in webkit/cgo). Build it with:
//
//	wails build -tags wails     // production bundle
//	wails dev   -tags wails     // live-reload dev
//
// The headless CLI lives in ./cmd/headless and needs no GUI toolchain.
func main() {
	fmt.Println("Build the GUI with: wails build -tags wails (or wails dev -tags wails)")
	fmt.Println("Headless CLI: go run ./cmd/headless -link 'vless://...'")
}
