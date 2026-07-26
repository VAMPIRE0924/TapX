package core

import "tapx/internal/config"

type standaloneDeviceHandle interface {
	Close() error
}

func standaloneRuntimeDevices(runtime *config.GeneratedRuntime) []config.RuntimeDevice {
	if runtime == nil {
		return nil
	}
	used := make(map[string]struct{})
	for _, pipe := range runtime.UDPPipes {
		used[pipe.DeviceID] = struct{}{}
	}
	for _, pipe := range runtime.TCPPipes {
		used[pipe.DeviceID] = struct{}{}
	}
	for _, pipe := range runtime.XrayPipes {
		used[pipe.DeviceID] = struct{}{}
	}
	out := make([]config.RuntimeDevice, 0, len(runtime.Devices))
	for _, device := range runtime.Devices {
		if _, ok := used[device.ID]; !ok {
			out = append(out, device)
		}
	}
	return out
}
