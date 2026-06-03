package core

import (
	"bytes"
	"fmt"

	xcore "github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"
	_ "github.com/xtls/xray-core/main/distro/all" // registers all protocols/transports
)

// Instance is a running xray-core instance.
type Instance struct {
	inner *xcore.Instance
}

// Start loads the JSON config and starts an xray-core instance.
func Start(jsonConfig []byte) (*Instance, error) {
	pbConfig, err := serial.LoadJSONConfig(bytes.NewReader(jsonConfig))
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	inst, err := xcore.New(pbConfig)
	if err != nil {
		return nil, fmt.Errorf("new instance: %w", err)
	}
	if err := inst.Start(); err != nil {
		return nil, fmt.Errorf("start instance: %w", err)
	}
	return &Instance{inner: inst}, nil
}

// Stop closes the instance and releases its resources.
func (i *Instance) Stop() error {
	if i == nil || i.inner == nil {
		return nil
	}
	return i.inner.Close()
}
