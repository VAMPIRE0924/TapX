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

func netapplyDNS(input config.RuntimeDNS) netapply.DNSConfig {
	return netapply.DNSConfig{
		Enabled:       input.Enabled,
		Nameservers:   append([]string(nil), input.Nameservers...),
		SearchDomains: append([]string(nil), input.SearchDomains...),
		Options:       append([]string(nil), input.Options...),
		OutputPath:    input.OutputPath,
	}
}
