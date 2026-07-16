//go:build !linux

package netapply

import "fmt"

type noopHandle struct{}

func ApplyDevice(cfg DeviceConfig) (Handle, error) {
	if !needsApply(cfg) {
		return noopHandle{}, nil
	}
	return nil, fmt.Errorf("netapply: device apply is only supported on linux")
}

func (noopHandle) SetMSSClamp(int, int) error {
	return fmt.Errorf("netapply: MSS clamp is only supported on linux")
}

func (noopHandle) Rollback() error { return nil }
