//go:build !linux && !windows

package netcfg

import "fmt"

func defaultGateway() (gw, dev string, err error) {
	return "", "", fmt.Errorf("default gateway discovery not supported on this OS")
}
