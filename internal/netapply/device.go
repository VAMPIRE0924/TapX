package netapply

import "tapx/internal/model"

type DeviceConfig struct {
	Type                  model.DeviceType
	IfName                string
	MTU                   int
	MSSClamp              int
	LinkAutoOptimize      bool
	Bridge                BridgeConfig
	Routes                []RouteConfig
	TapMode               model.TapMode
	AccessRole            model.AccessRole
	DHCP                  model.DHCPConfig
	SharedIP              model.SharedIPConfig
	TUNDHCP               model.TUNDHCPConfig
	AllowDefaultRoute     bool
	OneArmRollbackSeconds int
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
	ApplyAddressLease(lease AddressLease) error
	Rollback() error
}

type AddressLease struct {
	IPv4CIDR          string
	IPv6CIDR          string
	Gateway           string
	DNS               []string
	AllowDefaultRoute bool
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
		cfg.Bridge.Enabled ||
		hasEnabledRoutes(cfg.Routes) ||
		(cfg.DHCP.Mode != "" && cfg.DHCP.Mode != model.DHCPModeOff) ||
		(cfg.TUNDHCP.Mode != "" && cfg.TUNDHCP.Mode != model.TUNDHCPModeOff) ||
		cfg.TUNDHCP.RelayEnabled ||
		cfg.TapMode == model.TapModeSharedIP
}
