//go:build windows

package wintundll

import (
	_ "embed"
	"fmt"
	"sync"

	"golang.org/x/sys/windows"
)

// dllBytes is the embedded Wintun driver DLL (amd64). Replace via
// scripts/fetch-wintun.sh before building a release; the checked-in copy must
// be the genuine signed DLL from https://www.wintun.net.
//
//go:embed wintun_amd64.dll
var dllBytes []byte

const dllName = "wintun.dll"

// LOAD_WITH_ALTERED_SEARCH_PATH makes LoadLibraryEx resolve dependencies
// relative to the given absolute path rather than the process directory.
const loadWithAlteredSearchPath = 0x00000008

var ensureOnce struct {
	sync.Once
	err error
}

// Ensure extracts the embedded Wintun DLL and pre-loads it by absolute path.
// Once loaded, the wintun module's later LoadLibraryEx("wintun.dll", ...) call
// resolves to this already-loaded module (Windows matches by base name), so the
// DLL need not sit next to the executable. Idempotent and safe to call before
// every TUN start. Returning an error here prevents the log.Fatalf crash that
// would otherwise occur deep inside tun2socks when the DLL is unavailable.
func Ensure() error {
	ensureOnce.Do(func() {
		dir, err := DefaultDir()
		if err != nil {
			ensureOnce.err = fmt.Errorf("wintun: cache dir: %w", err)
			return
		}
		path, err := extract(dllBytes, dir, dllName)
		if err != nil {
			ensureOnce.err = fmt.Errorf("wintun: extract: %w", err)
			return
		}
		if _, err := windows.LoadLibraryEx(path, 0, loadWithAlteredSearchPath); err != nil {
			ensureOnce.err = fmt.Errorf("wintun: load %s: %w", path, err)
			return
		}
	})
	return ensureOnce.err
}
