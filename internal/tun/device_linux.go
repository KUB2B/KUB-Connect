package tun

import (
	"fmt"
	"os"
)

// checkDevice verifies the kernel TUN device node is present before tun2socks
// tries to open it (it would otherwise log.Fatalf and kill the process).
func checkDevice() error {
	if _, err := os.Stat("/dev/net/tun"); err != nil {
		return fmt.Errorf("tun: /dev/net/tun unavailable: %w", err)
	}
	return nil
}
