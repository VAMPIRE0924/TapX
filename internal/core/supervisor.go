package core

import (
	"context"
	"fmt"
	"net"
	"time"

	"tapx/internal/config"
	"tapx/internal/model"
	"tapx/internal/pathmtu"
	"tapx/internal/xrayruntime"
)

type Supervisor struct {
	udpPipes       []*UDPPipeHandle
	udpDispatches  []*udpReuseportGroup
	dtlsDispatches []*DTLSDispatchHandle
	tcpPipes       []*TCPPipeHandle
	tcpDispatches  []*TCPDispatchHandle
	xrayPipes      []*XrayPipeHandle
	xray           *xrayruntime.Manager
	externalXray   *xrayruntime.Manager
	pathMTU        *pathmtu.Cache
}

type ConnectorDiagnostic struct {
	Kind          string
	Transport     string
	Target        string
	Delay         time.Duration
	TCPMSS        int
	PathMTU       int
	UploadBytes   uint64
	DownloadBytes uint64
	UploadBPS     uint64
	DownloadBPS   uint64
	Duration      time.Duration
}

func NewSupervisor() *Supervisor {
	return &Supervisor{pathMTU: pathmtu.NewCache()}
}

func (s *Supervisor) Start(runtime *config.GeneratedRuntime) error {
	if runtime == nil {
		return fmt.Errorf("core: runtime is nil")
	}
	if len(s.udpPipes) != 0 || len(s.udpDispatches) != 0 || len(s.dtlsDispatches) != 0 || len(s.tcpPipes) != 0 || len(s.tcpDispatches) != 0 || len(s.xrayPipes) != 0 || s.xray != nil || s.externalXray != nil {
		return fmt.Errorf("core: supervisor already started")
	}
	if err := s.startTapX(runtime); err != nil {
		_ = s.Stop()
		return err
	}
	if err := s.startExternalXray(runtime); err != nil {
		_ = s.Stop()
		return err
	}
	if err := s.startEmbeddedXray(runtime); err != nil {
		_ = s.Stop()
		return err
	}
	return nil
}

const (
	RuntimeComponentTapX         = "tapx"
	RuntimeComponentEmbeddedXray = "embedded-xray"
	RuntimeComponentExternalXray = "external-xray"
)

func (s *Supervisor) RestartComponent(component string, runtime *config.GeneratedRuntime) error {
	if runtime == nil {
		return fmt.Errorf("core: runtime is nil")
	}
	switch component {
	case RuntimeComponentTapX:
		if err := s.stopTapX(); err != nil {
			return err
		}
		if err := s.startTapX(runtime); err != nil {
			_ = s.stopTapX()
			return err
		}
		return nil
	case RuntimeComponentEmbeddedXray:
		if err := s.stopEmbeddedXray(); err != nil {
			return err
		}
		if err := s.startEmbeddedXray(runtime); err != nil {
			_ = s.stopEmbeddedXray()
			return err
		}
		return nil
	case RuntimeComponentExternalXray:
		if err := s.stopExternalXray(); err != nil {
			return err
		}
		if err := s.startExternalXray(runtime); err != nil {
			_ = s.stopExternalXray()
			return err
		}
		return nil
	default:
		return fmt.Errorf("core: unsupported runtime component %q", component)
	}
}

func (s *Supervisor) StopComponent(component string) error {
	switch component {
	case RuntimeComponentTapX:
		return s.stopTapX()
	case RuntimeComponentEmbeddedXray:
		return s.stopEmbeddedXray()
	case RuntimeComponentExternalXray:
		return s.stopExternalXray()
	default:
		return fmt.Errorf("core: unsupported runtime component %q", component)
	}
}

func (s *Supervisor) startTapX(runtime *config.GeneratedRuntime) error {
	dtlsGroups := make(map[string]bool)
	for _, dispatch := range runtime.UDPDispatches {
		prototype, ok := findUDPDispatchPrototype(runtime.UDPPipes, dispatch.ID)
		if !ok {
			return fmt.Errorf("core: UDP dispatch %s has no worker pipe", dispatch.ID)
		}
		if prototype.DTLS.Enabled {
			handle, children, err := startDTLSDispatch(dispatch, runtime.UDPPipes, runtime.Devices, s.pathMTU)
			if err != nil {
				return err
			}
			dtlsGroups[dispatch.ID] = true
			s.dtlsDispatches = append(s.dtlsDispatches, handle)
			s.udpPipes = append(s.udpPipes, children...)
			continue
		}
		group, err := startUDPReuseportGroup(dispatch, prototype)
		if err != nil {
			return err
		}
		s.udpDispatches = append(s.udpDispatches, group)
	}
	for _, pipe := range runtime.UDPPipes {
		if dtlsGroups[pipe.DispatchGroup] {
			continue
		}
		device, ok := findRuntimeDevice(runtime.Devices, pipe.DeviceID)
		if !ok {
			return fmt.Errorf("core: udp pipe %s references missing device %s", pipe.EndpointID, pipe.DeviceID)
		}
		handle, err := startUDPPipeWithCache(pipe, device, s.pathMTU)
		if err != nil {
			return err
		}
		s.udpPipes = append(s.udpPipes, handle)
	}
	for _, dispatch := range runtime.TCPDispatches {
		handle, children, err := startTCPDispatch(dispatch, runtime.TCPPipes, runtime.Devices)
		if err != nil {
			return err
		}
		s.tcpDispatches = append(s.tcpDispatches, handle)
		s.tcpPipes = append(s.tcpPipes, children...)
	}
	for _, pipe := range runtime.TCPPipes {
		if pipe.DispatchGroup != "" || pipe.ExternalXrayBridge {
			continue
		}
		device, ok := findRuntimeDevice(runtime.Devices, pipe.DeviceID)
		if !ok {
			return fmt.Errorf("core: tcp pipe %s references missing device %s", pipe.EndpointID, pipe.DeviceID)
		}
		handle, err := startTCPPipe(pipe, device)
		if err != nil {
			return err
		}
		s.tcpPipes = append(s.tcpPipes, handle)
	}
	return nil
}

func (s *Supervisor) startEmbeddedXray(runtime *config.GeneratedRuntime) error {
	manager := xrayruntime.NewManager()
	devices := make(map[string]*xraySharedDevice)
	for _, pipe := range runtime.XrayPipes {
		if pipe.EndpointKind != "listener" || pipe.Runtime == model.XrayExternal || pipe.Action == model.RouteActionDrop {
			continue
		}
		device, ok := findRuntimeDevice(runtime.Devices, pipe.DeviceID)
		if !ok {
			return fmt.Errorf("core: xray pipe %s references missing device %s", pipe.EndpointID, pipe.DeviceID)
		}
		handle, err := startXrayPipeShared(pipe, device, manager, devices[pipe.DeviceID])
		if err != nil {
			return err
		}
		if handle.owner {
			devices[pipe.DeviceID] = handle.shared
		}
		s.xrayPipes = append(s.xrayPipes, handle)
	}
	if err := manager.Start(runtimeForXray(runtime, model.XrayEmbedded)); err != nil {
		_ = manager.Stop()
		return err
	}
	if manager.State().EndpointCount == 0 {
		return nil
	}
	s.xray = manager
	for _, pipe := range runtime.XrayPipes {
		if pipe.EndpointKind != "connector" || pipe.Runtime == model.XrayExternal || pipe.Action == model.RouteActionDrop {
			continue
		}
		device, ok := findRuntimeDevice(runtime.Devices, pipe.DeviceID)
		if !ok {
			return fmt.Errorf("core: xray pipe %s references missing device %s", pipe.EndpointID, pipe.DeviceID)
		}
		handle, err := startXrayPipeShared(pipe, device, manager, devices[pipe.DeviceID])
		if err != nil {
			return err
		}
		if handle.owner {
			devices[pipe.DeviceID] = handle.shared
		}
		s.xrayPipes = append(s.xrayPipes, handle)
	}
	return nil
}

func (s *Supervisor) startExternalXray(runtime *config.GeneratedRuntime) error {
	manager := xrayruntime.NewManager()
	devices := make(map[string]*tcpSharedDevice)
	for i := range runtime.TCPPipes {
		pipe := &runtime.TCPPipes[i]
		if !pipe.ExternalXrayBridge || pipe.EndpointKind != "connector" {
			continue
		}
		port, err := allocateLoopbackPort()
		if err != nil {
			return fmt.Errorf("core: allocate external xray bridge for %s: %w", pipe.EndpointID, err)
		}
		pipe.Remote = "127.0.0.1"
		pipe.Port = port
		setExternalBridgePort(runtime.Connectors, pipe.EndpointID, port)
	}
	for i := range runtime.TCPPipes {
		pipe := &runtime.TCPPipes[i]
		if !pipe.ExternalXrayBridge || pipe.EndpointKind != "listener" {
			continue
		}
		device, ok := findRuntimeDevice(runtime.Devices, pipe.DeviceID)
		if !ok {
			return fmt.Errorf("core: external xray bridge %s references missing device %s", pipe.EndpointID, pipe.DeviceID)
		}
		handle, err := startTCPPipeShared(*pipe, device, devices[pipe.DeviceID])
		if err != nil {
			return err
		}
		if handle.owner {
			devices[pipe.DeviceID] = handle.shared
		}
		s.tcpPipes = append(s.tcpPipes, handle)
		pipe.BindPort = handle.LocalAddr.Port()
		setExternalBridgePort(runtime.Listeners, pipe.EndpointID, handle.LocalAddr.Port())
	}
	if err := manager.Start(runtimeForXray(runtime, model.XrayExternal)); err != nil {
		_ = manager.Stop()
		return err
	}
	if manager.State().EndpointCount == 0 {
		return nil
	}
	s.externalXray = manager
	for _, pipe := range runtime.TCPPipes {
		if !pipe.ExternalXrayBridge || pipe.EndpointKind != "connector" {
			continue
		}
		device, ok := findRuntimeDevice(runtime.Devices, pipe.DeviceID)
		if !ok {
			return fmt.Errorf("core: external xray bridge %s references missing device %s", pipe.EndpointID, pipe.DeviceID)
		}
		handle, err := startTCPPipeShared(pipe, device, devices[pipe.DeviceID])
		if err != nil {
			return err
		}
		if handle.owner {
			devices[pipe.DeviceID] = handle.shared
		}
		s.tcpPipes = append(s.tcpPipes, handle)
	}
	return nil
}

func (s *Supervisor) stopTapX() error {
	var firstErr error
	for i := len(s.tcpDispatches) - 1; i >= 0; i-- {
		if err := s.tcpDispatches[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	s.tcpDispatches = nil
	for i := len(s.tcpPipes) - 1; i >= 0; i-- {
		if s.tcpPipes[i].Pipe.ExternalXrayBridge {
			continue
		}
		if err := s.tcpPipes[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		s.tcpPipes = append(s.tcpPipes[:i], s.tcpPipes[i+1:]...)
	}
	for i := len(s.dtlsDispatches) - 1; i >= 0; i-- {
		if err := s.dtlsDispatches[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	s.dtlsDispatches = nil
	for i := len(s.udpPipes) - 1; i >= 0; i-- {
		if err := s.udpPipes[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	s.udpPipes = nil
	for i := len(s.udpDispatches) - 1; i >= 0; i-- {
		if err := s.udpDispatches[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	s.udpDispatches = nil
	return firstErr
}

func (s *Supervisor) stopEmbeddedXray() error {
	var firstErr error
	if s.xray != nil {
		if err := s.xray.Stop(); err != nil {
			firstErr = err
		}
		s.xray = nil
	}
	for i := len(s.xrayPipes) - 1; i >= 0; i-- {
		if err := s.xrayPipes[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	s.xrayPipes = nil
	return firstErr
}

func (s *Supervisor) stopExternalXray() error {
	var firstErr error
	if s.externalXray != nil {
		if err := s.externalXray.Stop(); err != nil {
			firstErr = err
		}
		s.externalXray = nil
	}
	for i := len(s.tcpPipes) - 1; i >= 0; i-- {
		if !s.tcpPipes[i].Pipe.ExternalXrayBridge {
			continue
		}
		if err := s.tcpPipes[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		s.tcpPipes = append(s.tcpPipes[:i], s.tcpPipes[i+1:]...)
	}
	return firstErr
}

func (s *Supervisor) Stop() error {
	firstErr := s.stopExternalXray()
	if err := s.stopEmbeddedXray(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := s.stopTapX(); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func findUDPDispatchPrototype(pipes []config.RuntimeUDPPipe, groupID string) (config.RuntimeUDPPipe, bool) {
	for _, pipe := range pipes {
		if pipe.DispatchGroup == groupID {
			return pipe, true
		}
	}
	return config.RuntimeUDPPipe{}, false
}

func (s *Supervisor) CloseClientPipes(clientID string) (int, error) {
	if clientID == "" {
		return 0, nil
	}
	closed := 0
	var firstErr error
	dispatchEndpoints := make(map[string]bool)
	tcpDispatchEndpoints := make(map[string]bool)
	for _, pipe := range s.tcpPipes {
		if pipe.Pipe.Binding.ClientID == clientID && pipe.Pipe.DispatchGroup != "" {
			tcpDispatchEndpoints[pipe.Pipe.EndpointID] = true
		}
	}
	for _, pipe := range s.udpPipes {
		if pipe.Pipe.Binding.ClientID == clientID && pipe.Pipe.DispatchGroup != "" {
			dispatchEndpoints[pipe.Pipe.EndpointID] = true
		}
	}
	for endpointID := range tcpDispatchEndpoints {
		count, err := s.closeTCPDispatchEndpoint(endpointID)
		closed += count
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	for endpointID := range dispatchEndpoints {
		count, err := s.closeUDPDispatchEndpoint(endpointID)
		closed += count
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	for i := len(s.tcpPipes) - 1; i >= 0; i-- {
		if s.tcpPipes[i].Pipe.Binding.ClientID != clientID {
			continue
		}
		if s.tcpPipes[i].Pipe.DispatchGroup != "" {
			continue
		}
		if err := s.tcpPipes[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		s.tcpPipes = append(s.tcpPipes[:i], s.tcpPipes[i+1:]...)
		closed++
	}
	for i := len(s.udpPipes) - 1; i >= 0; i-- {
		if s.udpPipes[i].Pipe.Binding.ClientID != clientID {
			continue
		}
		if s.udpPipes[i].Pipe.DispatchGroup != "" {
			continue
		}
		if err := s.udpPipes[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		s.udpPipes = append(s.udpPipes[:i], s.udpPipes[i+1:]...)
		closed++
	}
	for i := len(s.xrayPipes) - 1; i >= 0; i-- {
		if s.xrayPipes[i].Pipe.Binding.ClientID != clientID {
			continue
		}
		if err := s.xrayPipes[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		s.xrayPipes = append(s.xrayPipes[:i], s.xrayPipes[i+1:]...)
		closed++
	}
	return closed, firstErr
}

func (s *Supervisor) CloseEndpointPipes(kind, endpointID string) (int, error) {
	if kind == "" || endpointID == "" {
		return 0, nil
	}
	closed := 0
	var firstErr error
	if kind == "listener" {
		count, err := s.closeTCPDispatchEndpoint(endpointID)
		closed += count
		if err != nil {
			firstErr = err
		}
		count, err = s.closeUDPDispatchEndpoint(endpointID)
		closed += count
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	for i := len(s.tcpPipes) - 1; i >= 0; i-- {
		if s.tcpPipes[i].Pipe.EndpointKind != kind || s.tcpPipes[i].Pipe.EndpointID != endpointID {
			continue
		}
		if err := s.tcpPipes[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		s.tcpPipes = append(s.tcpPipes[:i], s.tcpPipes[i+1:]...)
		closed++
	}
	for i := len(s.udpPipes) - 1; i >= 0; i-- {
		if s.udpPipes[i].Pipe.EndpointKind != kind || s.udpPipes[i].Pipe.EndpointID != endpointID {
			continue
		}
		if err := s.udpPipes[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		s.udpPipes = append(s.udpPipes[:i], s.udpPipes[i+1:]...)
		closed++
	}
	for i := len(s.xrayPipes) - 1; i >= 0; i-- {
		if s.xrayPipes[i].Pipe.EndpointKind != kind || s.xrayPipes[i].Pipe.EndpointID != endpointID {
			continue
		}
		if err := s.xrayPipes[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		s.xrayPipes = append(s.xrayPipes[:i], s.xrayPipes[i+1:]...)
		closed++
	}
	return closed, firstErr
}

func (s *Supervisor) closeUDPDispatchEndpoint(endpointID string) (int, error) {
	closed := 0
	var firstErr error
	for i := len(s.dtlsDispatches) - 1; i >= 0; i-- {
		if s.dtlsDispatches[i].Dispatch.EndpointID != endpointID {
			continue
		}
		if err := s.dtlsDispatches[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		s.dtlsDispatches = append(s.dtlsDispatches[:i], s.dtlsDispatches[i+1:]...)
	}
	for i := len(s.udpDispatches) - 1; i >= 0; i-- {
		if s.udpDispatches[i].Dispatch.EndpointID != endpointID {
			continue
		}
		if err := s.udpDispatches[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		s.udpDispatches = append(s.udpDispatches[:i], s.udpDispatches[i+1:]...)
	}
	for i := len(s.udpPipes) - 1; i >= 0; i-- {
		pipe := s.udpPipes[i]
		if pipe.Pipe.EndpointKind != "listener" || pipe.Pipe.EndpointID != endpointID || pipe.Pipe.DispatchGroup == "" {
			continue
		}
		if err := pipe.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		s.udpPipes = append(s.udpPipes[:i], s.udpPipes[i+1:]...)
		closed++
	}
	return closed, firstErr
}

func (s *Supervisor) closeTCPDispatchEndpoint(endpointID string) (int, error) {
	closed := 0
	var firstErr error
	for i := len(s.tcpDispatches) - 1; i >= 0; i-- {
		if s.tcpDispatches[i].Dispatch.EndpointID != endpointID {
			continue
		}
		if err := s.tcpDispatches[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		s.tcpDispatches = append(s.tcpDispatches[:i], s.tcpDispatches[i+1:]...)
	}
	for i := len(s.tcpPipes) - 1; i >= 0; i-- {
		pipe := s.tcpPipes[i]
		if pipe.Pipe.EndpointKind != "listener" || pipe.Pipe.EndpointID != endpointID || pipe.Pipe.DispatchGroup == "" {
			continue
		}
		if err := pipe.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		s.tcpPipes = append(s.tcpPipes[:i], s.tcpPipes[i+1:]...)
		closed++
	}
	return closed, firstErr
}

func (s *Supervisor) UDPPipes() []*UDPPipeHandle {
	out := make([]*UDPPipeHandle, len(s.udpPipes))
	copy(out, s.udpPipes)
	return out
}

func (s *Supervisor) TCPPipes() []*TCPPipeHandle {
	out := make([]*TCPPipeHandle, len(s.tcpPipes))
	copy(out, s.tcpPipes)
	return out
}

func (s *Supervisor) XrayPipes() []*XrayPipeHandle {
	out := make([]*XrayPipeHandle, len(s.xrayPipes))
	copy(out, s.xrayPipes)
	return out
}

func (s *Supervisor) XrayStates() []xrayruntime.State {
	states := make([]xrayruntime.State, 0, 2)
	if s.xray != nil {
		states = append(states, s.xray.States()...)
	}
	if s.externalXray != nil {
		states = append(states, s.externalXray.States()...)
	}
	return states
}

func (s *Supervisor) DialXrayTCP(ctx context.Context, outboundTag, host string, port uint16) (net.Conn, error) {
	if s.xray == nil {
		return nil, fmt.Errorf("core: embedded xray runtime is not running")
	}
	return s.xray.DialEmbeddedTCP(ctx, outboundTag, host, port)
}

func (s *Supervisor) DiagnoseConnector(ctx context.Context, endpointID, kind string, duration time.Duration) (ConnectorDiagnostic, error) {
	for _, pipe := range s.udpPipes {
		if pipe.Pipe.EndpointKind == "connector" && pipe.Pipe.EndpointID == endpointID {
			return pipe.Diagnose(ctx, kind, duration)
		}
	}
	for _, pipe := range s.tcpPipes {
		if pipe.Pipe.EndpointKind == "connector" && pipe.Pipe.EndpointID == endpointID {
			return pipe.Diagnose(ctx, kind, duration)
		}
	}
	for _, pipe := range s.xrayPipes {
		if pipe.Pipe.EndpointKind == "connector" && pipe.Pipe.EndpointID == endpointID {
			return pipe.Diagnose(ctx, kind, duration)
		}
	}
	return ConnectorDiagnostic{}, fmt.Errorf("core: connector %s has no diagnostic-capable active stream", endpointID)
}

func findRuntimeDevice(devices []config.RuntimeDevice, id string) (config.RuntimeDevice, bool) {
	for _, device := range devices {
		if device.ID == id {
			return device, true
		}
	}
	return config.RuntimeDevice{}, false
}

func runtimeForXray(runtime *config.GeneratedRuntime, target model.XrayRuntime) *config.GeneratedRuntime {
	if runtime == nil {
		return nil
	}
	filtered := *runtime
	profileIDs := make(map[string]struct{})
	filtered.XrayProfiles = nil
	for _, profile := range runtime.XrayProfiles {
		profileRuntime := profile.Runtime
		if profileRuntime == "" {
			profileRuntime = model.XrayEmbedded
		}
		if profileRuntime != target {
			continue
		}
		filtered.XrayProfiles = append(filtered.XrayProfiles, profile)
		profileIDs[profile.ID] = struct{}{}
	}
	filterEndpoints := func(endpoints []config.RuntimeEndpoint) []config.RuntimeEndpoint {
		out := make([]config.RuntimeEndpoint, 0, len(endpoints))
		for _, endpoint := range endpoints {
			if endpoint.Transport != model.TransportXray {
				continue
			}
			if _, ok := profileIDs[endpoint.XrayProfileID]; ok {
				out = append(out, endpoint)
			}
		}
		return out
	}
	filtered.Listeners = filterEndpoints(runtime.Listeners)
	filtered.Connectors = filterEndpoints(runtime.Connectors)
	filtered.XrayPipes = nil
	for _, pipe := range runtime.XrayPipes {
		pipeRuntime := pipe.Runtime
		if pipeRuntime == "" {
			pipeRuntime = model.XrayEmbedded
		}
		if pipeRuntime == target {
			filtered.XrayPipes = append(filtered.XrayPipes, pipe)
		}
	}
	filtered.UDPPipes = nil
	filtered.UDPDispatches = nil
	filtered.TCPDispatches = nil
	filtered.TCPPipes = nil
	if target == model.XrayExternal {
		for _, pipe := range runtime.TCPPipes {
			if pipe.ExternalXrayBridge {
				filtered.TCPPipes = append(filtered.TCPPipes, pipe)
			}
		}
	}
	return &filtered
}

func allocateLoopbackPort() (uint16, error) {
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		return 0, err
	}
	port := uint16(listener.Addr().(*net.TCPAddr).Port)
	if err := listener.Close(); err != nil {
		return 0, err
	}
	return port, nil
}

func setExternalBridgePort(endpoints []config.RuntimeEndpoint, endpointID string, port uint16) {
	for i := range endpoints {
		if endpoints[i].ID == endpointID {
			endpoints[i].ExternalBridgePort = port
			return
		}
	}
}
