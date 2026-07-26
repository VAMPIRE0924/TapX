//go:build linux

package core

import (
	"fmt"
	"net/netip"
	"sync"

	"golang.org/x/sys/unix"

	"tapx/internal/config"
	"tapx/internal/fastpath"
	"tapx/internal/model"
	"tapx/internal/netapply"
	"tapx/internal/tuntap"
)

type deviceFabricPortSpec struct {
	key      string
	deviceID string
	guard    config.RuntimeAddressGuard
	binding  config.RuntimeBinding
	endpoint string
}

type deviceFabricDevice struct {
	device    tuntap.Device
	netApply  netapply.Handle
	address   *deviceAddressControl
	worker    *fastpath.DeviceSwitch
	ports     []*sharedRuntimeDevice
	switchFDs []int
}

type deviceFabric struct {
	mu      sync.Mutex
	ports   map[string]*sharedRuntimeDevice
	devices []*deviceFabricDevice
}

func newDeviceFabric(runtime *config.GeneratedRuntime) (*deviceFabric, error) {
	fabric := &deviceFabric{ports: make(map[string]*sharedRuntimeDevice)}
	if runtime == nil {
		return fabric, nil
	}
	specs := collectDeviceFabricPorts(runtime)
	byDevice := make(map[string][]deviceFabricPortSpec)
	for _, spec := range specs {
		byDevice[spec.deviceID] = append(byDevice[spec.deviceID], spec)
	}
	for deviceID, deviceSpecs := range byDevice {
		if len(deviceSpecs) < 2 {
			continue
		}
		device, ok := findRuntimeDevice(runtime.Devices, deviceID)
		if !ok {
			_ = fabric.Close()
			return nil, fmt.Errorf("core: device fabric references missing device %s", deviceID)
		}
		if err := fabric.addDevice(device, deviceSpecs); err != nil {
			_ = fabric.Close()
			return nil, err
		}
	}
	return fabric, nil
}

func (f *deviceFabric) addDevice(device config.RuntimeDevice, specs []deviceFabricPortSpec) error {
	physical, netHandle, err := openAppliedDevice(device, true)
	if err != nil {
		return err
	}
	address, err := newDeviceAddressControl(device, netHandle)
	if err != nil {
		_ = netHandle.Rollback()
		_ = physical.Close()
		return err
	}
	entry := &deviceFabricDevice{device: physical, netApply: netHandle, address: address}
	switchPorts := make([]fastpath.DeviceSwitchPortConfig, 0, len(specs))
	claimedRoutes := make(map[string]string)
	for _, spec := range specs {
		pair, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_DGRAM|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, 0)
		if err != nil {
			entry.close()
			return fmt.Errorf("core: create device fabric port for %s: %w", device.ID, err)
		}
		for _, fd := range pair {
			_ = unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_SNDBUF, 4*1024*1024)
			_ = unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_RCVBUF, 4*1024*1024)
		}
		guardConfig := spec.guard
		if device.Type == model.DeviceTUN && address != nil && address.isServer() {
			ipv4, ipv6, leaseErr := address.serverLeasePrefixes(addressLeaseKey(spec.binding, device.ID, spec.endpoint))
			if leaseErr != nil {
				_ = unix.Close(pair[0])
				_ = unix.Close(pair[1])
				entry.close()
				return fmt.Errorf("core: reserve TUN lease for %s: %w", spec.key, leaseErr)
			}
			guardConfig.IPv4CIDRs = append(append([]string(nil), guardConfig.IPv4CIDRs...), ipv4...)
			guardConfig.IPv6CIDRs = append(append([]string(nil), guardConfig.IPv6CIDRs...), ipv6...)
		}
		if device.Type == model.DeviceTUN {
			if len(guardConfig.IPv4CIDRs) == 0 && len(guardConfig.IPv6CIDRs) == 0 {
				_ = unix.Close(pair[0])
				_ = unix.Close(pair[1])
				entry.close()
				return fmt.Errorf("core: TUN device %s has multiple channels but %s has no IP prefix or address lease", device.ID, spec.endpoint)
			}
			for _, prefix := range append(append([]string(nil), guardConfig.IPv4CIDRs...), guardConfig.IPv6CIDRs...) {
				parsed, parseErr := netip.ParsePrefix(prefix)
				if parseErr != nil {
					_ = unix.Close(pair[0])
					_ = unix.Close(pair[1])
					entry.close()
					return fmt.Errorf("core: invalid TUN prefix %q for %s: %w", prefix, spec.key, parseErr)
				}
				canonical := parsed.Masked().String()
				if previous := claimedRoutes[canonical]; previous != "" && previous != spec.key {
					_ = unix.Close(pair[0])
					_ = unix.Close(pair[1])
					entry.close()
					return fmt.Errorf("core: TUN prefix %s is assigned to both %s and %s", canonical, previous, spec.key)
				}
				claimedRoutes[canonical] = spec.key
			}
		}
		guard, err := fastpathAddressGuard(guardConfig)
		if err != nil {
			_ = unix.Close(pair[0])
			_ = unix.Close(pair[1])
			entry.close()
			return err
		}
		portDevice := tuntap.WrapFD(device.IfName, pair[1])
		shared := &sharedRuntimeDevice{device: portDevice, netApply: netHandle, address: address}
		entry.ports = append(entry.ports, shared)
		entry.switchFDs = append(entry.switchFDs, pair[0])
		f.ports[spec.key] = shared
		switchPorts = append(switchPorts, fastpath.DeviceSwitchPortConfig{FD: pair[0], Routes: guard})
	}
	frameKind, err := fastpath.FrameKindFromDevice(device.Type)
	if err != nil {
		entry.close()
		return err
	}
	entry.worker, err = fastpath.StartDeviceSwitch(fastpath.DeviceSwitchConfig{
		DeviceFD: physical.FD(), FrameKind: frameKind, MaxFrameSize: 65535, Ports: switchPorts,
	})
	if err != nil {
		entry.close()
		return fmt.Errorf("core: start %s fabric for %s: %w", device.Type, device.ID, err)
	}
	f.devices = append(f.devices, entry)
	return nil
}

func (f *deviceFabric) Port(key string) *sharedRuntimeDevice {
	if f == nil {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ports[key]
}

func (f *deviceFabric) Close() error {
	if f == nil {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var firstErr error
	for index := len(f.devices) - 1; index >= 0; index-- {
		if err := f.devices[index].close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	f.devices = nil
	f.ports = make(map[string]*sharedRuntimeDevice)
	return firstErr
}

func (d *deviceFabricDevice) close() error {
	if d == nil {
		return nil
	}
	var firstErr error
	if d.worker != nil {
		if err := d.worker.Stop(); err != nil {
			firstErr = err
		}
		d.worker = nil
	}
	for _, fd := range d.switchFDs {
		if err := unix.Close(fd); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	d.switchFDs = nil
	for _, port := range d.ports {
		if port != nil && port.device != nil {
			if err := port.device.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
			port.device = nil
		}
	}
	d.ports = nil
	if d.address != nil {
		d.address.Close()
		d.address = nil
	}
	if d.netApply != nil {
		if err := d.netApply.Rollback(); err != nil && firstErr == nil {
			firstErr = err
		}
		d.netApply = nil
	}
	if d.device != nil {
		if err := d.device.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		d.device = nil
	}
	return firstErr
}

func collectDeviceFabricPorts(runtime *config.GeneratedRuntime) []deviceFabricPortSpec {
	seen := make(map[string]struct{})
	out := make([]deviceFabricPortSpec, 0, len(runtime.UDPPipes)+len(runtime.TCPPipes)+len(runtime.XrayPipes))
	add := func(spec deviceFabricPortSpec) {
		if spec.deviceID == "" {
			return
		}
		if _, exists := seen[spec.key]; exists {
			return
		}
		seen[spec.key] = struct{}{}
		out = append(out, spec)
	}
	for _, pipe := range runtime.UDPPipes {
		add(deviceFabricPortSpec{key: udpFabricKey(pipe), deviceID: pipe.DeviceID, guard: pipe.AddressGuard, binding: pipe.Binding, endpoint: pipe.EndpointID})
	}
	for _, pipe := range runtime.TCPPipes {
		add(deviceFabricPortSpec{key: tcpFabricKey(pipe), deviceID: pipe.DeviceID, guard: pipe.AddressGuard, binding: pipe.Binding, endpoint: pipe.EndpointID})
	}
	for _, pipe := range runtime.XrayPipes {
		if pipe.Runtime != model.XrayExternal && pipe.Action != model.RouteActionDrop {
			add(deviceFabricPortSpec{key: xrayFabricKey(pipe), deviceID: pipe.DeviceID, guard: pipe.AddressGuard, binding: pipe.Binding, endpoint: pipe.EndpointID})
		}
	}
	return out
}
