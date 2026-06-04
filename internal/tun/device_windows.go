//go:build windows

package tun

import "github.com/zki/vless-client/internal/wintundll"

// checkDevice ensures the Wintun driver DLL is extracted and loaded before
// tun2socks tries to create the adapter. tun2socks would otherwise call
// log.Fatalf (killing the process) when the DLL cannot be found.
func checkDevice() error { return wintundll.Ensure() }
