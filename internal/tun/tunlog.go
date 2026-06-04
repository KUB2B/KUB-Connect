package tun

import (
	"io"

	t2log "github.com/xjasonlyu/tun2socks/v2/log"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// engineLogWriter, when set, receives the tun2socks engine's log output.
var engineLogWriter io.Writer

// SetLogWriter routes the tun2socks engine's logs to w. It must be called
// before Start. The GUI process has no usable stderr, so the engine's
// per-connection lines and device read errors are otherwise lost; capturing
// them is essential for diagnosing TUN-mode behaviour.
func SetLogWriter(w io.Writer) { engineLogWriter = w }

// installEngineLogger points the tun2socks global logger at w at Info level.
// engine.Start installs its own logger during startup, so this must run after
// engine.Start to take effect.
func installEngineLogger(w io.Writer) {
	enc := zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())
	core := zapcore.NewCore(enc, zapcore.AddSync(w), zapcore.InfoLevel)
	t2log.SetLogger(zap.New(core))
}
