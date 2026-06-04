package tun

import (
	"io"

	t2log "github.com/xjasonlyu/tun2socks/v2/log"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// engineLogWriter, when set, receives the tun2socks engine's log output.
var engineLogWriter io.Writer

// engineLevel is the xray-native verbosity (error/warning/debug) used for both
// the engine key and the captured logger. Defaults to warning.
var engineLevel = "warning"

// SetLogWriter routes the tun2socks engine's logs to w. It must be called
// before Start. The GUI process has no usable stderr, so the engine's
// per-connection lines and device read errors are otherwise lost.
func SetLogWriter(w io.Writer) { engineLogWriter = w }

// SetLogLevel sets the engine verbosity. Unknown values fall back to warning.
// Must be called before Start to take effect for a session.
func SetLogLevel(level string) { engineLevel = normalizeLevel(level) }

func normalizeLevel(level string) string {
	switch level {
	case "error", "warning", "debug":
		return level
	default:
		return "warning"
	}
}

func zapLevelFor(level string) zapcore.Level {
	switch level {
	case "debug":
		return zapcore.DebugLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.WarnLevel
	}
}

// installEngineLogger points the tun2socks global logger at w at the configured
// level. engine.Start installs its own logger during startup, so this must run
// after engine.Start to take effect.
func installEngineLogger(w io.Writer) {
	enc := zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())
	core := zapcore.NewCore(enc, zapcore.AddSync(w), zapLevelFor(engineLevel))
	t2log.SetLogger(zap.New(core))
}
