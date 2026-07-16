package config

import (
	"encoding/json"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"

	"tapx/internal/model"
)

type GeneratedRuntime struct {
	Devices       []RuntimeDevice
	Listeners     []RuntimeEndpoint
	Connectors    []RuntimeEndpoint
	Routes        []RuntimeRoute
	Clients       []model.Client
	XrayProfiles  []RuntimeXrayProfile
	Settings      []RuntimeSettings
	UDPPipes      []RuntimeUDPPipe
	UDPDispatches []RuntimeUDPDispatch
	TCPPipes      []RuntimeTCPPipe
	TCPDispatches []RuntimeTCPDispatch
	XrayPipes     []RuntimeXrayPipe
}

type RuntimeDevice struct {
	ID               string
	Type             model.DeviceType
	IfName           string
	MTU              int
	MSSClamp         int
	LinkAutoOptimize bool
	IPv4CIDR         string
	IPv6CIDR         string
	Bridge           RuntimeBridge
	Routes           []RuntimeDeviceRoute
	DNS              RuntimeDNS
}

type RuntimeBridge struct {
	Enabled bool
	Name    string
	IfName  string
	MTU     int
}

type RuntimeDeviceRoute struct {
	Enabled     bool
	Destination string
	Gateway     string
	Source      string
	IfName      string
	Metric      int
	Table       string
}

type RuntimeDNS struct {
	Enabled       bool
	Nameservers   []string
	SearchDomains []string
	Options       []string
	OutputPath    string
}

type RuntimeEndpoint struct {
	ID                 string
	Transport          model.Transport
	BindHost           string
	BindPort           uint16
	Remote             string
	Port               uint16
	XrayProfileID      string
	ExternalBridgePort uint16
	RawUDP             model.RawUDPSettings
	RawTCP             model.RawTCPSettings
	Binding            RuntimeBinding
	ExpiresAt          int64
	TrafficCap         uint64
	TrafficReset       string
}

type RuntimeRoute struct {
	ID           string
	Priority     int
	Action       model.RouteAction
	ListenerID   string
	DeviceID     string
	ConnectorID  string
	ClientID     string
	Binding      RuntimeBinding
	AddressGuard RuntimeAddressGuard
}

type RuntimeUDPPipe struct {
	EndpointID          string
	EndpointKind        string
	RouteID             string
	DeviceID            string
	FrameKind           model.DeviceType
	BindHost            string
	BindPort            uint16
	Remote              string
	Port                uint16
	PeerMode            model.UDPPeerMode
	FixedPeer           string
	BindInterface       string
	BindAddress         string
	ReceiveBuffer       int
	SendBuffer          int
	ReuseAddr           bool
	ReusePort           bool
	QueueSize           int
	ZeroCopy            bool
	ConnectTimeout      int
	IdleTimeout         int
	MaxFrameSize        int
	MaxDatagramPayload  int
	ConfirmedPathMTU    int
	EffectiveNetworkMTU int
	TCPMSSIPv4          int
	TCPMSSIPv6          int
	LinkAutoOptimize    bool
	DTLS                model.RawDTLSSettings
	AddressGuard        RuntimeAddressGuard
	AddressGuardRemote  bool
	Binding             RuntimeBinding
	DispatchGroup       string
	DispatchSocketIndex uint32
}

type RuntimeUDPDispatch struct {
	ID                  string
	EndpointID          string
	Routes              []RuntimeUDPDispatchRoute
	FallbackSocketIndex uint32
}

type RuntimeUDPDispatchRoute struct {
	VKeyValue   string
	SocketIndex uint32
}

type RuntimeTCPPipe struct {
	EndpointID         string
	EndpointKind       string
	RouteID            string
	DeviceID           string
	FrameKind          model.DeviceType
	BindHost           string
	BindPort           uint16
	Remote             string
	Port               uint16
	LengthMode         model.TCPLengthMode
	MaxFrameSize       int
	DeviceMTU          int
	TCPMaxSeg          int
	LinkAutoOptimize   bool
	BindInterface      string
	BindAddress        string
	ReceiveBuffer      int
	SendBuffer         int
	NoDelay            bool
	KeepAliveSecond    int
	FastOpen           bool
	ConnectTimeout     int
	ReconnectSecond    int
	QueueSize          int
	ZeroCopy           bool
	IdleTimeout        int
	AddressGuard       RuntimeAddressGuard
	AddressGuardRemote bool
	TLS                model.RawTLSSettings
	Binding            RuntimeBinding
	ExternalXrayBridge bool
	XrayBridgeTag      string
	XrayProfileID      string
	XrayRemote         string
	XrayPort           uint16
	DispatchGroup      string
	DispatchPolicyID   string
}

type RuntimeTCPDispatch struct {
	ID               string
	EndpointID       string
	Routes           []RuntimeTCPDispatchRoute
	FallbackPolicyID string
}

type RuntimeTCPDispatchRoute struct {
	VKeyValue string
	PolicyID  string
}

type RuntimeXrayPipe struct {
	EndpointID         string
	EndpointKind       string
	HandlerTag         string
	Priority           int
	Action             model.RouteAction
	ClientEmail        string
	Runtime            model.XrayRuntime
	RouteID            string
	DeviceID           string
	FrameKind          model.DeviceType
	XrayProfileID      string
	Remote             string
	Port               uint16
	LengthMode         model.TCPLengthMode
	MaxFrameSize       int
	DeviceMTU          int
	LinkAutoOptimize   bool
	AddressGuard       RuntimeAddressGuard
	AddressGuardRemote bool
	Binding            RuntimeBinding
}

type RuntimeAddressGuard struct {
	IPv4CIDRs []string
	IPv6CIDRs []string
	MACs      []string
}

type RuntimeBinding struct {
	VKeyValue         string
	ClientID          string
	RouteID           string
	DeviceID          string
	ConnectorID       string
	AddressID         string
	UploadRateLimit   uint64
	DownloadRateLimit uint64
}

type RuntimeXrayProfile struct {
	ID                   string
	Runtime              model.XrayRuntime
	InboundProtocol      string
	InboundSettingsJSON  string
	OutboundProtocol     string
	OutboundSettingsJSON string
	SendThrough          string
	TargetStrategy       string
	Network              string
	Security             string
	StreamSettingsJSON   string
	SniffingJSON         string
	MuxJSON              string
	SockoptJSON          string
	FallbacksJSON        string
	RoutingJSON          string
	DNSJSON              string
	PolicyJSON           string
	AdvancedJSON         string
}

type RuntimeSettings struct {
	ID                     string
	PanelListen            string
	PanelDomain            string
	PanelBasePath          string
	PanelHTTPS             bool
	PanelCertFile          string
	PanelKeyFile           string
	PanelAuthEnabled       bool
	AdminUsername          string
	SessionTTLSecond       int
	Timezone               string
	PanelOutbound          string
	ExternalXrayPath       string
	ExternalXrayConfigFile string
	ExternalXrayWorkDir    string
	ExternalXrayArgs       string
	LogLevel               string
	StatsIntervalSecond    int
	BackupDir              string
	DataDir                string
	OpenWrtBuildTarget     string
	AdvancedJSON           string
}

func GenerateRuntime(cfg RuntimeConfig) (*GeneratedRuntime, error) {
	if err := ValidateForApply(cfg); err != nil {
		return nil, err
	}

	index := runtimeIndex(cfg)
	kernels := configuredBuiltInKernels(cfg.Settings)
	out := &GeneratedRuntime{}
	now := time.Now().Unix()
	for _, item := range cfg.Clients {
		if !item.Enabled || (item.ExpiresAt > 0 && item.ExpiresAt <= now) {
			continue
		}
		out.Clients = append(out.Clients, item)
	}
	for _, item := range cfg.Devices {
		if !item.Enabled {
			continue
		}
		out.Devices = append(out.Devices, RuntimeDevice{
			ID:               item.ID,
			Type:             item.Type,
			IfName:           item.IfName,
			MTU:              item.MTU,
			MSSClamp:         item.MSSClamp,
			LinkAutoOptimize: item.LinkAutoOptimize,
			IPv4CIDR:         item.IPv4CIDR,
			IPv6CIDR:         item.IPv6CIDR,
			Bridge:           runtimeBridge(item.Bridge),
			Routes:           runtimeDeviceRoutes(item.Routes),
			DNS:              runtimeDNS(item.DNS),
		})
	}
	for _, item := range cfg.Routes {
		if !item.Enabled {
			continue
		}
		binding := index.routeBinding(item)
		out.Routes = append(out.Routes, RuntimeRoute{
			ID:           item.ID,
			Priority:     item.Priority,
			Action:       normalizeRouteAction(item.Action),
			ListenerID:   item.ListenerID,
			DeviceID:     item.DeviceID,
			ConnectorID:  item.ConnectorID,
			ClientID:     item.ClientID,
			Binding:      binding,
			AddressGuard: index.addressGuard(binding),
		})
	}
	sort.SliceStable(out.Routes, func(i, j int) bool {
		return out.Routes[i].Priority < out.Routes[j].Priority
	})
	for _, item := range cfg.XrayProfiles {
		if !item.Enabled {
			continue
		}
		out.XrayProfiles = append(out.XrayProfiles, RuntimeXrayProfile{
			ID:                   item.ID,
			Runtime:              normalizeXrayRuntime(item.Runtime),
			InboundProtocol:      item.InboundProtocol,
			InboundSettingsJSON:  item.InboundSettingsJSON,
			OutboundProtocol:     item.OutboundProtocol,
			OutboundSettingsJSON: item.OutboundSettingsJSON,
			SendThrough:          item.SendThrough,
			TargetStrategy:       item.TargetStrategy,
			Network:              item.Network,
			Security:             item.Security,
			StreamSettingsJSON:   item.StreamSettingsJSON,
			SniffingJSON:         item.SniffingJSON,
			MuxJSON:              item.MuxJSON,
			SockoptJSON:          item.SockoptJSON,
			FallbacksJSON:        item.FallbacksJSON,
			RoutingJSON:          item.RoutingJSON,
			DNSJSON:              item.DNSJSON,
			PolicyJSON:           item.PolicyJSON,
			AdvancedJSON:         item.AdvancedJSON,
		})
	}
	for _, item := range cfg.Settings {
		if !item.Enabled {
			continue
		}
		out.Settings = append(out.Settings, RuntimeSettings{
			ID:                     item.ID,
			PanelListen:            item.PanelListen,
			PanelDomain:            item.PanelDomain,
			PanelBasePath:          item.PanelBasePath,
			PanelHTTPS:             item.PanelHTTPS,
			PanelCertFile:          item.PanelCertFile,
			PanelKeyFile:           item.PanelKeyFile,
			PanelAuthEnabled:       item.PanelAuthEnabled,
			AdminUsername:          item.AdminUsername,
			SessionTTLSecond:       item.SessionTTLSecond,
			Timezone:               item.Timezone,
			PanelOutbound:          item.PanelOutbound,
			ExternalXrayPath:       item.ExternalXrayPath,
			ExternalXrayConfigFile: item.ExternalXrayConfigFile,
			ExternalXrayWorkDir:    item.ExternalXrayWorkDir,
			ExternalXrayArgs:       item.ExternalXrayArgs,
			LogLevel:               item.LogLevel,
			StatsIntervalSecond:    item.StatsIntervalSecond,
			BackupDir:              item.BackupDir,
			DataDir:                item.DataDir,
			OpenWrtBuildTarget:     item.OpenWrtBuildTarget,
			AdvancedJSON:           item.AdvancedJSON,
		})
	}
	for _, item := range cfg.Listeners {
		if !item.Enabled || (item.ExpiresAt > 0 && item.ExpiresAt <= now) {
			continue
		}
		if !kernels.allowsEndpoint(item.Transport, index.hasEmbeddedXrayProfile(item.XrayProfileID)) {
			continue
		}
		out.Listeners = append(out.Listeners, RuntimeEndpoint{
			ID:            item.ID,
			Transport:     item.Transport,
			BindHost:      item.BindHost,
			BindPort:      item.BindPort,
			XrayProfileID: item.XrayProfileID,
			RawUDP:        item.RawUDP,
			RawTCP:        item.RawTCP,
			Binding:       index.binding(item.Binding),
			ExpiresAt:     item.ExpiresAt,
			TrafficCap:    item.TrafficCap,
			TrafficReset:  item.TrafficReset,
		})
		if item.Transport == model.TransportUDP {
			pipes, dispatch := index.udpPipesFromListener(item, out.Routes, now)
			out.UDPPipes = append(out.UDPPipes, pipes...)
			if dispatch != nil {
				out.UDPDispatches = append(out.UDPDispatches, *dispatch)
			}
		} else if item.Transport == model.TransportTCP {
			pipes, dispatch := index.tcpPipesFromListener(item, out.Routes, now)
			out.TCPPipes = append(out.TCPPipes, pipes...)
			if dispatch != nil {
				out.TCPDispatches = append(out.TCPDispatches, *dispatch)
			}
		} else if item.Transport == model.TransportXray {
			policies := index.xrayPipesFromListener(item, out.Routes, now)
			out.XrayPipes = append(out.XrayPipes, policies...)
			if index.hasExternalXrayProfile(item.XrayProfileID) {
				for _, policy := range policies {
					if policy.Action == model.RouteActionDrop {
						continue
					}
					if pipe, ok := index.externalXrayBridgeFromPolicy(item, policy); ok {
						out.TCPPipes = append(out.TCPPipes, pipe)
					}
				}
			}
		}
	}
	for _, item := range cfg.Connectors {
		if !item.Enabled {
			continue
		}
		if !kernels.allowsEndpoint(item.Transport, index.hasEmbeddedXrayProfile(item.XrayProfileID)) {
			continue
		}
		out.Connectors = append(out.Connectors, RuntimeEndpoint{
			ID:            item.ID,
			Transport:     item.Transport,
			Remote:        item.Remote,
			Port:          item.Port,
			XrayProfileID: item.XrayProfileID,
			RawUDP:        item.RawUDP,
			RawTCP:        item.RawTCP,
			Binding:       index.binding(item.Binding),
		})
		if item.Transport == model.TransportUDP {
			if pipe, ok := index.udpPipeFromConnector(item); ok {
				out.UDPPipes = append(out.UDPPipes, pipe)
			}
		} else if item.Transport == model.TransportTCP {
			if pipe, ok := index.tcpPipeFromConnector(item); ok {
				out.TCPPipes = append(out.TCPPipes, pipe)
			}
		} else if item.Transport == model.TransportXray {
			if pipe, ok := index.xrayPipeFromConnector(item); ok {
				out.XrayPipes = append(out.XrayPipes, pipe)
			} else if pipe, ok := index.externalXrayBridgeFromConnector(item); ok {
				out.TCPPipes = append(out.TCPPipes, pipe)
			}
		}
	}
	return out, nil
}

type builtInKernelSettings struct {
	embeddedXray bool
	tapx         bool
}

func configuredBuiltInKernels(settings []model.Settings) builtInKernelSettings {
	configured := builtInKernelSettings{embeddedXray: true, tapx: true}
	for _, item := range settings {
		if !item.Enabled || strings.TrimSpace(item.AdvancedJSON) == "" {
			continue
		}
		var values struct {
			EmbeddedXrayEnabled *bool `json:"embeddedXrayEnabled"`
			TapxEnabled         *bool `json:"tapxEnabled"`
		}
		if json.Unmarshal([]byte(item.AdvancedJSON), &values) != nil {
			continue
		}
		if values.EmbeddedXrayEnabled != nil {
			configured.embeddedXray = *values.EmbeddedXrayEnabled
		}
		if values.TapxEnabled != nil {
			configured.tapx = *values.TapxEnabled
		}
		break
	}
	return configured
}

func (settings builtInKernelSettings) allowsEndpoint(transport model.Transport, embeddedXray bool) bool {
	switch transport {
	case model.TransportUDP, model.TransportTCP:
		return settings.tapx
	case model.TransportXray:
		return !embeddedXray || settings.embeddedXray
	default:
		return true
	}
}

type generatorIndex struct {
	devices      map[string]model.Device
	routes       map[string]model.Route
	clients      map[string]model.Client
	vkeys        map[string]model.VKey
	addresses    map[string]model.AddressLimit
	xrayProfiles map[string]model.XrayProfile
}

func runtimeIndex(cfg RuntimeConfig) generatorIndex {
	idx := generatorIndex{
		devices:      make(map[string]model.Device, len(cfg.Devices)),
		routes:       make(map[string]model.Route, len(cfg.Routes)),
		clients:      make(map[string]model.Client, len(cfg.Clients)),
		vkeys:        make(map[string]model.VKey, len(cfg.VKeys)),
		addresses:    make(map[string]model.AddressLimit, len(cfg.Addresses)),
		xrayProfiles: make(map[string]model.XrayProfile, len(cfg.XrayProfiles)),
	}
	for _, item := range cfg.Devices {
		idx.devices[item.ID] = item
	}
	for _, item := range cfg.Routes {
		idx.routes[item.ID] = item
	}
	for _, item := range cfg.Clients {
		idx.clients[item.ID] = item
	}
	for _, item := range cfg.VKeys {
		idx.vkeys[item.ID] = item
	}
	for _, item := range cfg.Addresses {
		idx.addresses[item.ID] = item
	}
	for _, item := range cfg.XrayProfiles {
		idx.xrayProfiles[item.ID] = item
	}
	return idx
}

func (idx generatorIndex) udpPipeFromListenerBinding(input model.Listener, binding RuntimeBinding) (RuntimeUDPPipe, bool) {
	device, ok := idx.deviceForBinding(binding)
	if !ok {
		return RuntimeUDPPipe{}, false
	}
	return RuntimeUDPPipe{
		EndpointID:         input.ID,
		EndpointKind:       "listener",
		RouteID:            binding.RouteID,
		DeviceID:           device.ID,
		FrameKind:          device.Type,
		BindHost:           input.BindHost,
		BindPort:           input.BindPort,
		PeerMode:           normalizeUDPPeerMode(input.RawUDP.PeerMode),
		FixedPeer:          input.RawUDP.FixedPeer,
		BindInterface:      input.RawUDP.BindInterface,
		BindAddress:        input.RawUDP.BindAddress,
		ReceiveBuffer:      input.RawUDP.ReceiveBuffer,
		SendBuffer:         input.RawUDP.SendBuffer,
		ReuseAddr:          input.RawUDP.ReuseAddr,
		ReusePort:          input.RawUDP.ReusePort,
		QueueSize:          input.RawUDP.QueueSize,
		ZeroCopy:           input.RawUDP.ZeroCopy,
		ConnectTimeout:     input.RawUDP.ConnectTimeout,
		IdleTimeout:        input.RawUDP.IdleTimeout,
		MaxFrameSize:       maxFrameSize(device),
		LinkAutoOptimize:   device.LinkAutoOptimize,
		DTLS:               input.RawUDP.DTLS,
		AddressGuard:       idx.addressGuard(binding),
		AddressGuardRemote: idx.listenerAddressGuardRemote(input.ID, binding),
		Binding:            binding,
	}, true
}

type rawListenerPolicy struct {
	priority int
	action   model.RouteAction
	binding  RuntimeBinding
	guard    RuntimeAddressGuard
}

func (idx generatorIndex) udpPipesFromListener(input model.Listener, routes []RuntimeRoute, now int64) ([]RuntimeUDPPipe, *RuntimeUDPDispatch) {
	policies := idx.rawListenerPolicies(input, routes, now)
	if len(policies) == 0 {
		return nil, nil
	}

	type compiledPolicy struct {
		policy rawListenerPolicy
		pipe   RuntimeUDPPipe
	}
	compiled := make([]compiledPolicy, 0, len(policies))
	dropKeys := make([]string, 0)
	seenKeys := make(map[string]bool)
	fallbackSeen := false
	for _, policy := range policies {
		key := policy.binding.VKeyValue
		if key == "" {
			if fallbackSeen {
				continue
			}
			fallbackSeen = true
		} else if seenKeys[key] {
			continue
		} else {
			seenKeys[key] = true
		}
		if policy.action == model.RouteActionDrop {
			if key != "" {
				dropKeys = append(dropKeys, key)
			}
			continue
		}
		pipe, ok := idx.udpPipeFromListenerBinding(input, policy.binding)
		if !ok {
			continue
		}
		pipe.AddressGuard = policy.guard
		pipe.AddressGuardRemote = key != "" || idx.listenerAddressGuardRemote(input.ID, policy.binding)
		compiled = append(compiled, compiledPolicy{policy: policy, pipe: pipe})
	}
	if len(compiled) == 0 {
		return nil, nil
	}
	dispatchRequired := input.RawUDP.DTLS.Enabled || len(compiled) > 1 || len(dropKeys) > 0
	if !dispatchRequired {
		return []RuntimeUDPPipe{compiled[0].pipe}, nil
	}

	groupID := "raw-udp-" + input.ID
	dispatch := &RuntimeUDPDispatch{ID: groupID, EndpointID: input.ID}
	pipes := make([]RuntimeUDPPipe, 0, len(compiled))
	for _, item := range compiled {
		pipe := item.pipe
		pipe.ReusePort = true
		pipe.DispatchGroup = groupID
		pipe.DispatchSocketIndex = uint32(len(pipes) + 1) // socket zero is the kernel drop sink
		pipes = append(pipes, pipe)
		if item.policy.binding.VKeyValue == "" {
			dispatch.FallbackSocketIndex = pipe.DispatchSocketIndex
		} else {
			dispatch.Routes = append(dispatch.Routes, RuntimeUDPDispatchRoute{
				VKeyValue: item.policy.binding.VKeyValue, SocketIndex: pipe.DispatchSocketIndex,
			})
		}
	}
	for _, key := range dropKeys {
		dispatch.Routes = append(dispatch.Routes, RuntimeUDPDispatchRoute{VKeyValue: key})
	}
	return pipes, dispatch
}

func (idx generatorIndex) rawListenerPolicies(input model.Listener, routes []RuntimeRoute, now int64) []rawListenerPolicy {
	base := idx.binding(input.Binding)
	policies := make([]rawListenerPolicy, 0)
	explicitClients := make(map[string]bool)
	for _, route := range routes {
		if route.ListenerID != "" && route.ListenerID != input.ID {
			continue
		}
		if route.ClientID != "" {
			client, ok := idx.clients[route.ClientID]
			if !ok || !clientActiveAt(client, now) || !clientAllowsListenerRuntime(client, input.ID) {
				continue
			}
			explicitClients[route.ClientID] = true
		} else if route.ListenerID == "" && route.Binding.VKeyValue == "" {
			continue
		}
		binding := route.Binding
		if binding.DeviceID == "" && route.Action == model.RouteActionAllow {
			binding.DeviceID = base.DeviceID
		}
		policies = append(policies, rawListenerPolicy{
			priority: route.Priority, action: route.Action, binding: binding, guard: route.AddressGuard,
		})
	}
	for _, client := range idx.clients {
		if explicitClients[client.ID] || !clientActiveAt(client, now) || !clientAllowsListenerRuntime(client, input.ID) {
			continue
		}
		binding := idx.clientRuntimeBinding(client)
		if binding.VKeyValue == "" || binding.DeviceID == "" {
			continue
		}
		policies = append(policies, rawListenerPolicy{
			priority: 100, action: model.RouteActionBindDevice, binding: binding, guard: idx.addressGuard(binding),
		})
	}
	if base.DeviceID != "" {
		policies = append(policies, rawListenerPolicy{
			priority: int(^uint(0) >> 1), action: model.RouteActionBindDevice, binding: base, guard: idx.addressGuard(base),
		})
	}
	sort.SliceStable(policies, func(i, j int) bool { return policies[i].priority < policies[j].priority })
	return policies
}

func (idx generatorIndex) udpPipeFromConnector(input model.Connector) (RuntimeUDPPipe, bool) {
	binding := idx.binding(input.Binding)
	device, ok := idx.deviceForBinding(binding)
	if !ok {
		return RuntimeUDPPipe{}, false
	}
	return RuntimeUDPPipe{
		EndpointID:         input.ID,
		EndpointKind:       "connector",
		RouteID:            binding.RouteID,
		DeviceID:           device.ID,
		FrameKind:          device.Type,
		Remote:             input.Remote,
		Port:               input.Port,
		PeerMode:           normalizeUDPPeerMode(input.RawUDP.PeerMode),
		FixedPeer:          input.RawUDP.FixedPeer,
		BindInterface:      input.RawUDP.BindInterface,
		BindAddress:        input.RawUDP.BindAddress,
		ReceiveBuffer:      input.RawUDP.ReceiveBuffer,
		SendBuffer:         input.RawUDP.SendBuffer,
		ReuseAddr:          input.RawUDP.ReuseAddr,
		ReusePort:          input.RawUDP.ReusePort,
		QueueSize:          input.RawUDP.QueueSize,
		ZeroCopy:           input.RawUDP.ZeroCopy,
		ConnectTimeout:     input.RawUDP.ConnectTimeout,
		IdleTimeout:        input.RawUDP.IdleTimeout,
		MaxFrameSize:       maxFrameSize(device),
		LinkAutoOptimize:   device.LinkAutoOptimize,
		DTLS:               input.RawUDP.DTLS,
		AddressGuard:       idx.addressGuard(binding),
		AddressGuardRemote: false,
		Binding:            binding,
	}, true
}

func (idx generatorIndex) tcpPipeFromListenerBinding(input model.Listener, binding RuntimeBinding) (RuntimeTCPPipe, bool) {
	device, ok := idx.deviceForBinding(binding)
	if !ok {
		return RuntimeTCPPipe{}, false
	}
	return RuntimeTCPPipe{
		EndpointID:         input.ID,
		EndpointKind:       "listener",
		RouteID:            binding.RouteID,
		DeviceID:           device.ID,
		FrameKind:          device.Type,
		BindHost:           input.BindHost,
		BindPort:           input.BindPort,
		LengthMode:         normalizeTCPLengthMode(input.RawTCP.LengthMode),
		MaxFrameSize:       maxFrameSize(device),
		DeviceMTU:          device.MTU,
		TCPMaxSeg:          automaticTCPMaxSeg(device, first(input.RawTCP.BindAddress, input.BindHost)),
		LinkAutoOptimize:   device.LinkAutoOptimize,
		BindInterface:      input.RawTCP.BindInterface,
		BindAddress:        input.RawTCP.BindAddress,
		ReceiveBuffer:      input.RawTCP.ReceiveBuffer,
		SendBuffer:         input.RawTCP.SendBuffer,
		NoDelay:            input.RawTCP.NoDelay,
		KeepAliveSecond:    input.RawTCP.KeepAliveSecond,
		FastOpen:           input.RawTCP.FastOpen,
		ConnectTimeout:     input.RawTCP.ConnectTimeout,
		ReconnectSecond:    input.RawTCP.ReconnectSecond,
		QueueSize:          input.RawTCP.QueueSize,
		ZeroCopy:           input.RawTCP.ZeroCopy,
		IdleTimeout:        input.RawTCP.IdleTimeout,
		AddressGuard:       idx.addressGuard(binding),
		AddressGuardRemote: idx.listenerAddressGuardRemote(input.ID, binding),
		TLS:                input.RawTCP.TLS,
		Binding:            binding,
	}, true
}

func (idx generatorIndex) tcpPipesFromListener(input model.Listener, routes []RuntimeRoute, now int64) ([]RuntimeTCPPipe, *RuntimeTCPDispatch) {
	policies := idx.rawListenerPolicies(input, routes, now)
	if len(policies) == 0 {
		return nil, nil
	}
	type compiledPolicy struct {
		policy rawListenerPolicy
		pipe   RuntimeTCPPipe
	}
	compiled := make([]compiledPolicy, 0, len(policies))
	dropKeys := make([]string, 0)
	seenKeys := make(map[string]bool)
	fallbackSeen := false
	for _, policy := range policies {
		key := policy.binding.VKeyValue
		if key == "" {
			if fallbackSeen {
				continue
			}
			fallbackSeen = true
		} else if seenKeys[key] {
			continue
		} else {
			seenKeys[key] = true
		}
		if policy.action == model.RouteActionDrop {
			if key != "" {
				dropKeys = append(dropKeys, key)
			}
			continue
		}
		pipe, ok := idx.tcpPipeFromListenerBinding(input, policy.binding)
		if !ok {
			continue
		}
		pipe.AddressGuard = policy.guard
		pipe.AddressGuardRemote = key != "" || idx.listenerAddressGuardRemote(input.ID, policy.binding)
		compiled = append(compiled, compiledPolicy{policy: policy, pipe: pipe})
	}
	if len(compiled) == 0 {
		return nil, nil
	}
	if len(compiled) == 1 && len(dropKeys) == 0 {
		return []RuntimeTCPPipe{compiled[0].pipe}, nil
	}

	groupID := "raw-tcp-" + input.ID
	dispatch := &RuntimeTCPDispatch{ID: groupID, EndpointID: input.ID}
	pipes := make([]RuntimeTCPPipe, 0, len(compiled))
	for index, item := range compiled {
		pipe := item.pipe
		pipe.DispatchGroup = groupID
		pipe.DispatchPolicyID = groupID + "-policy-" + strconv.Itoa(index+1)
		pipes = append(pipes, pipe)
		if item.policy.binding.VKeyValue == "" {
			dispatch.FallbackPolicyID = pipe.DispatchPolicyID
		} else {
			dispatch.Routes = append(dispatch.Routes, RuntimeTCPDispatchRoute{
				VKeyValue: item.policy.binding.VKeyValue, PolicyID: pipe.DispatchPolicyID,
			})
		}
	}
	for _, key := range dropKeys {
		dispatch.Routes = append(dispatch.Routes, RuntimeTCPDispatchRoute{VKeyValue: key})
	}
	return pipes, dispatch
}

func (idx generatorIndex) tcpPipeFromConnector(input model.Connector) (RuntimeTCPPipe, bool) {
	binding := idx.binding(input.Binding)
	device, ok := idx.deviceForBinding(binding)
	if !ok {
		return RuntimeTCPPipe{}, false
	}
	return RuntimeTCPPipe{
		EndpointID:         input.ID,
		EndpointKind:       "connector",
		RouteID:            binding.RouteID,
		DeviceID:           device.ID,
		FrameKind:          device.Type,
		Remote:             input.Remote,
		Port:               input.Port,
		LengthMode:         normalizeTCPLengthMode(input.RawTCP.LengthMode),
		MaxFrameSize:       maxFrameSize(device),
		DeviceMTU:          device.MTU,
		TCPMaxSeg:          automaticTCPMaxSeg(device, input.Remote),
		LinkAutoOptimize:   device.LinkAutoOptimize,
		BindInterface:      input.RawTCP.BindInterface,
		BindAddress:        input.RawTCP.BindAddress,
		ReceiveBuffer:      input.RawTCP.ReceiveBuffer,
		SendBuffer:         input.RawTCP.SendBuffer,
		NoDelay:            input.RawTCP.NoDelay,
		KeepAliveSecond:    input.RawTCP.KeepAliveSecond,
		FastOpen:           input.RawTCP.FastOpen,
		ConnectTimeout:     input.RawTCP.ConnectTimeout,
		ReconnectSecond:    input.RawTCP.ReconnectSecond,
		QueueSize:          input.RawTCP.QueueSize,
		ZeroCopy:           input.RawTCP.ZeroCopy,
		IdleTimeout:        input.RawTCP.IdleTimeout,
		AddressGuard:       idx.addressGuard(binding),
		AddressGuardRemote: false,
		TLS:                input.RawTCP.TLS,
		Binding:            binding,
	}, true
}

func (idx generatorIndex) xrayPipesFromListener(input model.Listener, routes []RuntimeRoute, now int64) []RuntimeXrayPipe {
	base := idx.binding(input.Binding)
	out := make([]RuntimeXrayPipe, 0)
	hasExplicitClientRoute := make(map[string]bool)

	for _, route := range routes {
		if route.ListenerID != "" && route.ListenerID != input.ID {
			continue
		}
		if route.ClientID != "" {
			client, ok := idx.clients[route.ClientID]
			if !ok || !clientActiveAt(client, now) || !clientAllowsListenerRuntime(client, input.ID) {
				continue
			}
			hasExplicitClientRoute[client.ID] = true
		} else if route.ListenerID == "" {
			// A route without a listener or client selector does not implicitly
			// capture every Xray listener. It remains available to raw/device
			// rule evaluation where vKey and source identity can identify it.
			continue
		}
		if route.Binding.VKeyValue != "" {
			continue
		}
		binding := route.Binding
		if binding.DeviceID == "" && route.Action == model.RouteActionAllow {
			binding.DeviceID = base.DeviceID
		}
		out = append(out, idx.xrayListenerPolicy(input, binding, route.AddressGuard, route.Priority, route.Action, route.ClientID))
	}

	for _, client := range idx.clients {
		if !clientActiveAt(client, now) || !clientAllowsListenerRuntime(client, input.ID) || hasExplicitClientRoute[client.ID] {
			continue
		}
		binding := idx.clientRuntimeBinding(client)
		if binding.DeviceID == "" || binding.VKeyValue != "" {
			continue
		}
		out = append(out, idx.xrayListenerPolicy(input, binding, idx.addressGuard(binding), 100, model.RouteActionBindDevice, client.ID))
	}

	if base.DeviceID != "" {
		out = append(out, idx.xrayListenerPolicy(input, base, idx.addressGuard(base), int(^uint(0)>>1), model.RouteActionBindDevice, ""))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Priority < out[j].Priority })
	return out
}

func (idx generatorIndex) xrayListenerPolicy(input model.Listener, binding RuntimeBinding, guard RuntimeAddressGuard, priority int, action model.RouteAction, clientID string) RuntimeXrayPipe {
	profile := idx.xrayProfiles[input.XrayProfileID]
	pipe := RuntimeXrayPipe{
		EndpointID:         input.ID,
		EndpointKind:       "listener",
		HandlerTag:         runtimeXrayHandlerTag(input.ID, binding.RouteID, clientID, binding.DeviceID),
		Priority:           priority,
		Action:             action,
		ClientEmail:        idx.clientEmail(clientID),
		Runtime:            normalizeXrayRuntime(profile.Runtime),
		RouteID:            binding.RouteID,
		DeviceID:           binding.DeviceID,
		XrayProfileID:      input.XrayProfileID,
		LengthMode:         normalizeTCPLengthMode(input.RawTCP.LengthMode),
		AddressGuard:       guard,
		AddressGuardRemote: clientID != "" || idx.listenerAddressGuardRemote(input.ID, binding),
		Binding:            binding,
	}
	if device, ok := idx.deviceForBinding(binding); ok {
		pipe.FrameKind = device.Type
		pipe.MaxFrameSize = maxFrameSize(device)
		pipe.DeviceMTU = device.MTU
		pipe.LinkAutoOptimize = device.LinkAutoOptimize
	}
	return pipe
}

func (idx generatorIndex) externalXrayBridgeFromPolicy(input model.Listener, policy RuntimeXrayPipe) (RuntimeTCPPipe, bool) {
	device, ok := idx.deviceForBinding(policy.Binding)
	if !ok {
		return RuntimeTCPPipe{}, false
	}
	binding := policy.Binding
	binding.VKeyValue = ""
	return RuntimeTCPPipe{
		EndpointID:         input.ID,
		EndpointKind:       "listener",
		RouteID:            policy.RouteID,
		DeviceID:           device.ID,
		FrameKind:          device.Type,
		BindHost:           "127.0.0.1",
		BindPort:           0,
		LengthMode:         normalizeTCPLengthMode(input.RawTCP.LengthMode),
		MaxFrameSize:       maxFrameSize(device),
		DeviceMTU:          device.MTU,
		NoDelay:            true,
		KeepAliveSecond:    input.RawTCP.KeepAliveSecond,
		QueueSize:          input.RawTCP.QueueSize,
		ZeroCopy:           input.RawTCP.ZeroCopy,
		IdleTimeout:        input.RawTCP.IdleTimeout,
		AddressGuard:       policy.AddressGuard,
		AddressGuardRemote: policy.AddressGuardRemote,
		Binding:            binding,
		ExternalXrayBridge: true,
		XrayBridgeTag:      policy.HandlerTag,
		XrayProfileID:      input.XrayProfileID,
	}, true
}

func (idx generatorIndex) xrayPipeFromConnector(input model.Connector) (RuntimeXrayPipe, bool) {
	if !idx.hasEmbeddedXrayProfile(input.XrayProfileID) {
		return RuntimeXrayPipe{}, false
	}
	binding := idx.binding(input.Binding)
	device, ok := idx.deviceForBinding(binding)
	if !ok {
		return RuntimeXrayPipe{}, false
	}
	return RuntimeXrayPipe{
		EndpointID:         input.ID,
		EndpointKind:       "connector",
		HandlerTag:         input.ID,
		Action:             model.RouteActionBindDevice,
		Runtime:            model.XrayEmbedded,
		RouteID:            binding.RouteID,
		DeviceID:           device.ID,
		FrameKind:          device.Type,
		XrayProfileID:      input.XrayProfileID,
		Remote:             input.Remote,
		Port:               input.Port,
		LengthMode:         normalizeTCPLengthMode(input.RawTCP.LengthMode),
		MaxFrameSize:       maxFrameSize(device),
		DeviceMTU:          device.MTU,
		LinkAutoOptimize:   device.LinkAutoOptimize,
		AddressGuard:       idx.addressGuard(binding),
		AddressGuardRemote: false,
		Binding:            binding,
	}, true
}

func (idx generatorIndex) externalXrayBridgeFromConnector(input model.Connector) (RuntimeTCPPipe, bool) {
	if !idx.hasExternalXrayProfile(input.XrayProfileID) {
		return RuntimeTCPPipe{}, false
	}
	binding := idx.binding(input.Binding)
	device, ok := idx.deviceForBinding(binding)
	if !ok {
		return RuntimeTCPPipe{}, false
	}
	binding.VKeyValue = ""
	return RuntimeTCPPipe{
		EndpointID:         input.ID,
		EndpointKind:       "connector",
		RouteID:            binding.RouteID,
		DeviceID:           device.ID,
		FrameKind:          device.Type,
		Remote:             "127.0.0.1",
		LengthMode:         normalizeTCPLengthMode(input.RawTCP.LengthMode),
		MaxFrameSize:       maxFrameSize(device),
		DeviceMTU:          device.MTU,
		NoDelay:            true,
		KeepAliveSecond:    input.RawTCP.KeepAliveSecond,
		ConnectTimeout:     max(input.RawTCP.ConnectTimeout, 5),
		QueueSize:          input.RawTCP.QueueSize,
		ZeroCopy:           input.RawTCP.ZeroCopy,
		IdleTimeout:        input.RawTCP.IdleTimeout,
		AddressGuard:       idx.addressGuard(binding),
		AddressGuardRemote: false,
		Binding:            binding,
		ExternalXrayBridge: true,
		XrayProfileID:      input.XrayProfileID,
		XrayRemote:         input.Remote,
		XrayPort:           input.Port,
	}, true
}

func (idx generatorIndex) hasEmbeddedXrayProfile(id string) bool {
	profile, ok := idx.xrayProfiles[id]
	return ok && profile.Enabled && normalizeXrayRuntime(profile.Runtime) == model.XrayEmbedded
}

func (idx generatorIndex) hasExternalXrayProfile(id string) bool {
	profile, ok := idx.xrayProfiles[id]
	return ok && profile.Enabled && normalizeXrayRuntime(profile.Runtime) == model.XrayExternal
}

func (idx generatorIndex) deviceForBinding(binding RuntimeBinding) (model.Device, bool) {
	if binding.DeviceID == "" {
		return model.Device{}, false
	}
	device, ok := idx.devices[binding.DeviceID]
	if !ok || !device.Enabled {
		return model.Device{}, false
	}
	return device, true
}

func (idx generatorIndex) binding(input model.Binding) RuntimeBinding {
	out := idx.bindingBase(input)
	client, ok := idx.clients[out.ClientID]
	if !ok || !client.Enabled {
		return out
	}
	clientBinding := idx.bindingBase(client.Binding)
	out.VKeyValue = first(out.VKeyValue, clientBinding.VKeyValue)
	out.RouteID = first(out.RouteID, clientBinding.RouteID)
	out.DeviceID = first(out.DeviceID, clientBinding.DeviceID)
	out.ConnectorID = first(out.ConnectorID, clientBinding.ConnectorID)
	out.AddressID = first(out.AddressID, client.AddressID, clientBinding.AddressID)
	out.UploadRateLimit = client.UploadRateLimit
	out.DownloadRateLimit = client.DownloadRateLimit
	return out
}

func (idx generatorIndex) routeBinding(route model.Route) RuntimeBinding {
	out := RuntimeBinding{
		VKeyValue:   idx.vkeyValue(route.VKeyID),
		ClientID:    route.ClientID,
		RouteID:     route.ID,
		DeviceID:    route.DeviceID,
		ConnectorID: route.ConnectorID,
		AddressID:   route.AddressID,
	}
	client, ok := idx.clients[route.ClientID]
	if !ok || !client.Enabled {
		return out
	}
	clientBinding := idx.bindingBase(client.Binding)
	out.VKeyValue = first(out.VKeyValue, clientBinding.VKeyValue)
	out.DeviceID = first(out.DeviceID, clientBinding.DeviceID)
	out.ConnectorID = first(out.ConnectorID, clientBinding.ConnectorID)
	out.AddressID = first(out.AddressID, client.AddressID, clientBinding.AddressID)
	out.UploadRateLimit = client.UploadRateLimit
	out.DownloadRateLimit = client.DownloadRateLimit
	return out
}

func (idx generatorIndex) clientRuntimeBinding(client model.Client) RuntimeBinding {
	out := idx.bindingBase(client.Binding)
	out.ClientID = client.ID
	out.AddressID = first(client.AddressID, out.AddressID)
	out.UploadRateLimit = client.UploadRateLimit
	out.DownloadRateLimit = client.DownloadRateLimit
	return out
}

func (idx generatorIndex) clientEmail(id string) string {
	if id == "" {
		return ""
	}
	client, ok := idx.clients[id]
	if !ok {
		return ""
	}
	return first(client.Email, client.ID)
}

func clientActiveAt(client model.Client, now int64) bool {
	return client.Enabled && (client.ExpiresAt <= 0 || client.ExpiresAt > now)
}

func clientAllowsListenerRuntime(client model.Client, listenerID string) bool {
	hasAssignment := false
	if assigned := strings.TrimSpace(client.ListenerID); assigned != "" {
		hasAssignment = true
		if assigned == listenerID {
			return true
		}
	}
	for _, assigned := range client.ListenerIDs {
		assigned = strings.TrimSpace(assigned)
		if assigned == "" {
			continue
		}
		hasAssignment = true
		if assigned == listenerID {
			return true
		}
	}
	return !hasAssignment
}

func runtimeXrayHandlerTag(endpointID, routeID, clientID, deviceID string) string {
	if routeID == "" && clientID == "" {
		return "tapx-frame-" + endpointID
	}
	parts := []string{"tapx-frame", endpointID}
	for _, value := range []string{routeID, clientID, deviceID} {
		value = strings.TrimSpace(value)
		if value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, "-")
}

func (idx generatorIndex) bindingBase(input model.Binding) RuntimeBinding {
	out := RuntimeBinding{
		VKeyValue:   idx.vkeyValue(input.VKeyID),
		ClientID:    input.ClientID,
		RouteID:     input.RouteID,
		DeviceID:    input.DeviceID,
		ConnectorID: input.ConnectorID,
		AddressID:   input.AddressID,
	}
	if input.RouteID == "" {
		return out
	}
	route, ok := idx.routes[input.RouteID]
	if !ok {
		return out
	}
	out.VKeyValue = first(out.VKeyValue, idx.vkeyValue(route.VKeyID))
	out.ClientID = first(out.ClientID, route.ClientID)
	out.DeviceID = first(out.DeviceID, route.DeviceID)
	out.ConnectorID = first(out.ConnectorID, route.ConnectorID)
	out.AddressID = first(out.AddressID, route.AddressID)
	return out
}

func (idx generatorIndex) addressGuard(binding RuntimeBinding) RuntimeAddressGuard {
	if binding.AddressID == "" {
		return RuntimeAddressGuard{}
	}
	address, ok := idx.addresses[binding.AddressID]
	if !ok || !address.Enabled {
		return RuntimeAddressGuard{}
	}
	return RuntimeAddressGuard{
		IPv4CIDRs: append([]string(nil), address.IPv4CIDRs...),
		IPv6CIDRs: append([]string(nil), address.IPv6CIDRs...),
		MACs:      append([]string(nil), address.MACs...),
	}
}

func (idx generatorIndex) listenerAddressGuardRemote(listenerID string, binding RuntimeBinding) bool {
	if binding.ClientID != "" || binding.VKeyValue != "" {
		return true
	}
	route, ok := idx.routes[binding.RouteID]
	if !ok {
		return false
	}
	return route.ListenerID == listenerID || route.ClientID != "" || route.VKeyID != ""
}

func normalizeUDPPeerMode(mode model.UDPPeerMode) model.UDPPeerMode {
	if mode == "" {
		return model.UDPPeerAny
	}
	return mode
}

func normalizeTCPLengthMode(mode model.TCPLengthMode) model.TCPLengthMode {
	if mode == "" {
		return model.TCPLength16
	}
	return mode
}

func automaticTCPMaxSeg(device model.Device, outer string) int {
	if !device.LinkAutoOptimize || device.MTU <= 0 {
		return 0
	}
	header := 60
	if addr, err := netip.ParseAddr(strings.TrimSpace(outer)); err == nil && addr.Is4() {
		header = 40
	}
	mss := device.MTU - header
	if mss < 536 {
		return 0
	}
	return mss
}

func normalizeXrayRuntime(runtime model.XrayRuntime) model.XrayRuntime {
	if runtime == "" {
		return model.XrayEmbedded
	}
	return runtime
}

func normalizeRouteAction(action model.RouteAction) model.RouteAction {
	if action == "" {
		return model.RouteActionBindDevice
	}
	return action
}

func runtimeBridge(input *model.BridgeConfig) RuntimeBridge {
	if input == nil {
		return RuntimeBridge{}
	}
	return RuntimeBridge{
		Enabled: input.Enabled,
		Name:    input.Name,
		IfName:  input.IfName,
		MTU:     input.MTU,
	}
}

func runtimeDeviceRoutes(input []model.DeviceRoute) []RuntimeDeviceRoute {
	if len(input) == 0 {
		return nil
	}
	out := make([]RuntimeDeviceRoute, 0, len(input))
	for _, item := range input {
		if !item.Enabled {
			continue
		}
		out = append(out, RuntimeDeviceRoute{
			Enabled:     item.Enabled,
			Destination: item.Destination,
			Gateway:     item.Gateway,
			Source:      item.Source,
			IfName:      item.IfName,
			Metric:      item.Metric,
			Table:       item.Table,
		})
	}
	return out
}

func runtimeDNS(input *model.DNSConfig) RuntimeDNS {
	if input == nil {
		return RuntimeDNS{}
	}
	return RuntimeDNS{
		Enabled:       input.Enabled,
		Nameservers:   append([]string(nil), input.Nameservers...),
		SearchDomains: append([]string(nil), input.SearchDomains...),
		Options:       append([]string(nil), input.Options...),
		OutputPath:    input.OutputPath,
	}
}

func maxFrameSize(device model.Device) int {
	if device.MTU > 0 {
		if device.Type == model.DeviceTAP {
			// TAP includes the Ethernet header. Reserve two VLAN tags so a
			// full-MTU 802.1Q/QinQ frame is not truncated (FCS is omitted).
			return device.MTU + 22
		}
		return device.MTU
	}
	return 65535
}

func (idx generatorIndex) vkeyValue(id string) string {
	if id == "" {
		return ""
	}
	item, ok := idx.vkeys[id]
	if !ok || !item.Enabled {
		return ""
	}
	return item.Value
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
