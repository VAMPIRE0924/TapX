//go:build linux

package core

import (
	"tapx/internal/config"
	"tapx/internal/netapply"
)

func netapplyRoutes(input []config.RuntimeDeviceRoute) []netapply.RouteConfig {
	if len(input) == 0 {
		return nil
	}
	out := make([]netapply.RouteConfig, 0, len(input))
	for _, route := range input {
		if !route.Enabled {
			continue
		}
		out = append(out, netapply.RouteConfig{
			Enabled:     route.Enabled,
			Destination: route.Destination,
			Gateway:     route.Gateway,
			Source:      route.Source,
			IfName:      route.IfName,
			Metric:      route.Metric,
			Table:       route.Table,
		})
	}
	return out
}

func netapplyDeviceConfig(device config.RuntimeDevice, ifName string) netapply.DeviceConfig {
	return netapply.DeviceConfig{
		Type: device.Type, IfName: ifName, MTU: device.MTU, MSSClamp: device.MSSClamp,
		LinkAutoOptimize: device.LinkAutoOptimize,
		Bridge:           netapply.BridgeConfig{Enabled: device.Bridge.Enabled, Name: device.Bridge.Name, IfName: device.Bridge.IfName, MTU: device.Bridge.MTU},
		Routes:           netapplyRoutes(device.Routes),
		TapMode:          device.TapMode, AccessRole: device.AccessRole, DHCP: device.DHCP,
		SharedIP: device.SharedIP, TUNDHCP: device.TUNDHCP, AllowDefaultRoute: device.AllowDefaultRoute,
		OneArmRollbackSeconds: device.OneArmRollbackSeconds,
	}
}
