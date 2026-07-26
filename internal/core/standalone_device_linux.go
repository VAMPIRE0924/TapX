//go:build linux

package core

import (
	"fmt"

	"tapx/internal/config"
	"tapx/internal/netapply"
	"tapx/internal/tuntap"
)

type linuxStandaloneDevice struct {
	device   tuntap.Device
	netApply netapply.Handle
}

func (h *linuxStandaloneDevice) Close() error {
	if h == nil {
		return nil
	}
	var firstErr error
	if h.netApply != nil {
		firstErr = h.netApply.Rollback()
	}
	if h.device != nil {
		if err := h.device.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *Supervisor) startStandaloneDevices(runtime *config.GeneratedRuntime) error {
	for _, device := range standaloneRuntimeDevices(runtime) {
		tunDevice, err := tuntap.Open(tuntap.OpenOptions{Name: device.IfName, Type: device.Type, NonBlock: true})
		if err != nil {
			return fmt.Errorf("core: open standalone %s %s: %w", device.Type, device.IfName, err)
		}
		netHandle, err := netapply.ApplyDevice(netapplyDeviceConfig(device, tunDevice.Name()))
		if err != nil {
			_ = tunDevice.Close()
			return fmt.Errorf("core: apply standalone device %s: %w", tunDevice.Name(), err)
		}
		s.standaloneDevices = append(s.standaloneDevices, &linuxStandaloneDevice{device: tunDevice, netApply: netHandle})
	}
	return nil
}
