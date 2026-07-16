package netapply

import "tapx/internal/model"

type DeviceConfig struct {
	Type             model.DeviceType
	IfName           string
	MTU              int
	MSSClamp         int
	LinkAutoOptimize bool
	IPv4CIDR         string
	IPv6CIDR         string
	Bridge           BridgeConfig
	Routes           []RouteConfig
	DNS              DNSConfig
}

type BridgeConfig struct {
	Enabled bool
	Name    string
	IfName  string
	MTU     int
}

type RouteConfig struct {
	Enabled     bool
	Destination string
	Gateway     string
	Source      string
	IfName      string
	Metric      int
	Table       string
}

type DNSConfig struct {
	Enabled       bool
	Nameservers   []string
	SearchDomains []string
	Options       []string
	OutputPath    string
}

type Handle interface {
	SetMSSClamp(ipv4MSS, ipv6MSS int) error
	Rollback() error
}

func hasEnabledRoutes(routes []RouteConfig) bool {
	for _, route := range routes {
		if route.Enabled {
			return true
		}
	}
	return false
}

func needsApply(cfg DeviceConfig) bool {
	return cfg.MTU > 0 ||
		cfg.MSSClamp > 0 ||
		cfg.LinkAutoOptimize ||
		cfg.IPv4CIDR != "" ||
		cfg.IPv6CIDR != "" ||
		cfg.Bridge.Enabled ||
		hasEnabledRoutes(cfg.Routes) ||
		cfg.DNS.Enabled
}
