package config

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"

	"tapx/internal/model"
)

type RawPairTemplateOptions struct {
	Transport model.Transport
	HostA     string
	HostB     string
	Port      uint16
	TunA      string
	TunB      string
	IfNameA   string
	IfNameB   string
	MTU       int
	VKey      string
}

type RawPairTemplate struct {
	Transport model.Transport   `json:"transport"`
	HostA     string            `json:"hostA"`
	HostB     string            `json:"hostB"`
	Port      uint16            `json:"port"`
	A         RuntimeConfig     `json:"a"`
	B         RuntimeConfig     `json:"b"`
	RuntimeA  *GeneratedRuntime `json:"runtimeA,omitempty"`
	RuntimeB  *GeneratedRuntime `json:"runtimeB,omitempty"`
}

func BuildRawPairTemplate(options RawPairTemplateOptions) (RawPairTemplate, error) {
	options = normalizeRawPairOptions(options)
	if err := validateRawPairOptions(options); err != nil {
		return RawPairTemplate{}, err
	}

	var a RuntimeConfig
	var b RuntimeConfig
	switch options.Transport {
	case model.TransportUDP:
		a = rawUDPPeerConfig("a", options.IfNameA, options.TunA, options.HostB, options.Port, options.MTU, options.VKey)
		b = rawUDPPeerConfig("b", options.IfNameB, options.TunB, options.HostA, options.Port, options.MTU, options.VKey)
	case model.TransportTCP:
		a = rawTCPListenerConfig(options.IfNameA, options.TunA, options.Port, options.MTU, options.VKey)
		b = rawTCPConnectorConfig(options.IfNameB, options.TunB, options.HostA, options.Port, options.MTU, options.VKey)
	default:
		return RawPairTemplate{}, fmt.Errorf("transport must be udp or tcp")
	}

	runtimeA, err := GenerateRuntime(a)
	if err != nil {
		return RawPairTemplate{}, fmt.Errorf("generate side A runtime: %w", err)
	}
	runtimeB, err := GenerateRuntime(b)
	if err != nil {
		return RawPairTemplate{}, fmt.Errorf("generate side B runtime: %w", err)
	}

	return RawPairTemplate{
		Transport: options.Transport,
		HostA:     options.HostA,
		HostB:     options.HostB,
		Port:      options.Port,
		A:         a,
		B:         b,
		RuntimeA:  runtimeA,
		RuntimeB:  runtimeB,
	}, nil
}

func normalizeRawPairOptions(in RawPairTemplateOptions) RawPairTemplateOptions {
	if in.Transport == "" {
		in.Transport = model.TransportUDP
	}
	if in.Port == 0 {
		if in.Transport == model.TransportTCP {
			in.Port = 46001
		} else {
			in.Port = 46000
		}
	}
	if in.TunA == "" {
		if in.Transport == model.TransportTCP {
			in.TunA = "10.78.0.1/30"
		} else {
			in.TunA = "10.77.0.1/30"
		}
	}
	if in.TunB == "" {
		if in.Transport == model.TransportTCP {
			in.TunB = "10.78.0.2/30"
		} else {
			in.TunB = "10.77.0.2/30"
		}
	}
	if in.IfNameA == "" {
		if in.Transport == model.TransportTCP {
			in.IfNameA = "tapxtcp0"
		} else {
			in.IfNameA = "tapxudp0"
		}
	}
	if in.IfNameB == "" {
		if in.Transport == model.TransportTCP {
			in.IfNameB = "tapxtcp0"
		} else {
			in.IfNameB = "tapxudp0"
		}
	}
	if in.MTU == 0 {
		in.MTU = 1500
	}
	return in
}

func validateRawPairOptions(options RawPairTemplateOptions) error {
	switch options.Transport {
	case model.TransportUDP, model.TransportTCP:
	default:
		return fmt.Errorf("transport must be udp or tcp")
	}
	if options.HostA == "" || options.HostB == "" {
		return fmt.Errorf("hostA and hostB are required")
	}
	if _, err := netip.ParseAddr(options.HostA); err != nil {
		return fmt.Errorf("hostA must be an IP address: %w", err)
	}
	if _, err := netip.ParseAddr(options.HostB); err != nil {
		return fmt.Errorf("hostB must be an IP address: %w", err)
	}
	if options.Port == 0 {
		return fmt.Errorf("port is required")
	}
	if _, err := netip.ParsePrefix(options.TunA); err != nil {
		return fmt.Errorf("tunA must be CIDR: %w", err)
	}
	if _, err := netip.ParsePrefix(options.TunB); err != nil {
		return fmt.Errorf("tunB must be CIDR: %w", err)
	}
	if strings.TrimSpace(options.IfNameA) == "" || strings.TrimSpace(options.IfNameB) == "" {
		return fmt.Errorf("ifNameA and ifNameB are required")
	}
	if options.MTU < 576 || options.MTU > 65535 {
		return fmt.Errorf("mtu must be between 576 and 65535")
	}
	return nil
}

func rawUDPPeerConfig(side, ifName, cidr, peerHost string, port uint16, mtu int, vkey string) RuntimeConfig {
	deviceID := "tun-" + side
	routeID := "route-" + side
	listenerID := "udp-" + side
	cfg := baseRawTemplateConfig(deviceID, routeID, ifName, cidr, mtu, vkey)
	cfg.Listeners = []model.Listener{{
		ID:        listenerID,
		Enabled:   true,
		Name:      "Raw UDP " + strings.ToUpper(side),
		BindHost:  "0.0.0.0",
		BindPort:  port,
		Transport: model.TransportUDP,
		RawUDP: model.RawUDPSettings{
			PeerMode:      model.UDPPeerFixed,
			FixedPeer:     net.JoinHostPort(peerHost, strconv.Itoa(int(port))),
			ReuseAddr:     true,
			ReceiveBuffer: 1048576,
			SendBuffer:    1048576,
			ZeroCopy:      true,
		},
		Binding: model.Binding{RouteID: routeID},
	}}
	return cfg
}

func rawTCPListenerConfig(ifName, cidr string, port uint16, mtu int, vkey string) RuntimeConfig {
	cfg := baseRawTemplateConfig("tun-a", "route-a", ifName, cidr, mtu, vkey)
	cfg.Listeners = []model.Listener{{
		ID:        "tcp-a",
		Enabled:   true,
		Name:      "Raw TCP Listener",
		BindHost:  "0.0.0.0",
		BindPort:  port,
		Transport: model.TransportTCP,
		RawTCP:    rawTCPTemplateSettings(),
		Binding:   model.Binding{RouteID: "route-a"},
	}}
	return cfg
}

func rawTCPConnectorConfig(ifName, cidr, remote string, port uint16, mtu int, vkey string) RuntimeConfig {
	cfg := baseRawTemplateConfig("tun-b", "route-b", ifName, cidr, mtu, vkey)
	cfg.Connectors = []model.Connector{{
		ID:        "tcp-b",
		Enabled:   true,
		Name:      "Raw TCP Connector",
		Remote:    remote,
		Port:      port,
		Transport: model.TransportTCP,
		RawTCP:    rawTCPTemplateSettings(),
		Binding:   model.Binding{RouteID: "route-b"},
	}}
	return cfg
}

func baseRawTemplateConfig(deviceID, routeID, ifName, cidr string, mtu int, vkey string) RuntimeConfig {
	cfg := RuntimeConfig{
		Devices: []model.Device{{
			ID:       deviceID,
			Enabled:  true,
			Name:     deviceID,
			Type:     model.DeviceTUN,
			IfName:   ifName,
			MTU:      mtu,
			IPv4CIDR: cidr,
		}},
		Routes: []model.Route{{
			ID:       routeID,
			Enabled:  true,
			DeviceID: deviceID,
		}},
	}
	if vkey != "" {
		cfg.VKeys = []model.VKey{{
			ID:      "raw-vkey",
			Enabled: true,
			Name:    "Raw vKey",
			Value:   vkey,
		}}
		cfg.Routes[0].VKeyID = "raw-vkey"
	}
	return cfg
}

func rawTCPTemplateSettings() model.RawTCPSettings {
	return model.RawTCPSettings{
		LengthMode:      model.TCPLength16,
		ReceiveBuffer:   1048576,
		SendBuffer:      1048576,
		NoDelay:         true,
		KeepAliveSecond: 30,
		ConnectTimeout:  5,
		ZeroCopy:        true,
	}
}
