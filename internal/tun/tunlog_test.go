package tun

import (
	"testing"

	"go.uber.org/zap/zapcore"
)

func TestZapLevelFor(t *testing.T) {
	cases := map[string]zapcore.Level{
		"debug":   zapcore.DebugLevel,
		"warning": zapcore.WarnLevel,
		"error":   zapcore.ErrorLevel,
		"":        zapcore.WarnLevel,
		"bogus":   zapcore.WarnLevel,
	}
	for in, want := range cases {
		if got := zapLevelFor(in); got != want {
			t.Errorf("zapLevelFor(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestSetLogLevelUpdatesEngineLevel(t *testing.T) {
	t.Cleanup(func() { engineLevel = "warning" })
	SetLogLevel("debug")
	if engineLevel != "debug" {
		t.Errorf("engineLevel = %q, want debug", engineLevel)
	}
	SetLogLevel("nonsense")
	if engineLevel != "warning" {
		t.Errorf("engineLevel = %q, want warning (coerced)", engineLevel)
	}
}
