//go:build windows

package netcfg

type windowsRouter struct{}

// New returns the Windows (netsh) router.
func New() Router { return windowsRouter{} }

// Supported reports whether TUN routing is implemented on this OS.
func Supported() bool { return true }

func (windowsRouter) Up(c Config) error   { return runAll(winUpCommands(c)) }
func (windowsRouter) Down(c Config) error { return runAll(winDownCommands(c)) }
