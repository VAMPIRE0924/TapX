package panel

import (
	"fmt"
	"sync"
	"time"

	"tapx/internal/config"
	"tapx/internal/core"
	"tapx/internal/fastpath"
	"tapx/internal/model"
	"tapx/internal/xrayruntime"
)

type RuntimeController interface {
	Start(*config.GeneratedRuntime) error
	Stop() error
	CloseClientPipes(clientID string) (int, error)
	UDPPipes() []*core.UDPPipeHandle
	TCPPipes() []*core.TCPPipeHandle
	XrayPipes() []*core.XrayPipeHandle
	XrayStates() []xrayruntime.State
}

type RuntimeManager struct {
	mu                sync.Mutex
	newController     func() RuntimeController
	controller        RuntimeController
	activeRuntime     *config.GeneratedRuntime
	activeConfig      *config.RuntimeConfig
	generation        uint64
	startedAt         time.Time
	stoppedAt         time.Time
	lastAppliedAt     time.Time
	lastRollbackAt    time.Time
	lastEnforcedAt    time.Time
	lastRollbackError string
	lastReloadMode    string
	lastError         string
	enforcementStop   chan struct{}
	enforcementDone   chan struct{}
	enforcementEvents []ClientEnforcementEvent
}

type RuntimeState struct {
	Running           bool                     `json:"running"`
	Generation        uint64                   `json:"generation"`
	StartedAt         string                   `json:"startedAt,omitempty"`
	StoppedAt         string                   `json:"stoppedAt,omitempty"`
	LastAppliedAt     string                   `json:"lastAppliedAt,omitempty"`
	LastRollbackAt    string                   `json:"lastRollbackAt,omitempty"`
	LastEnforcedAt    string                   `json:"lastEnforcedAt,omitempty"`
	LastRollbackError string                   `json:"lastRollbackError,omitempty"`
	LastReloadMode    string                   `json:"lastReloadMode,omitempty"`
	LastError         string                   `json:"lastError,omitempty"`
	EnforcementEvents []ClientEnforcementEvent `json:"enforcementEvents,omitempty"`
	UDPPipes          []RuntimePipeState       `json:"udpPipes"`
	TCPPipes          []RuntimePipeState       `json:"tcpPipes"`
	XrayPipes         []RuntimePipeState       `json:"xrayPipes"`
	XrayRuntimes      []xrayruntime.State      `json:"xrayRuntimes,omitempty"`
}

type RuntimePipeState struct {
	EndpointID   string                    `json:"endpointId"`
	EndpointKind string                    `json:"endpointKind"`
	Transport    string                    `json:"transport"`
	RouteID      string                    `json:"routeId,omitempty"`
	DeviceID     string                    `json:"deviceId"`
	DeviceName   string                    `json:"deviceName,omitempty"`
	ClientID     string                    `json:"clientId,omitempty"`
	AddressID    string                    `json:"addressId,omitempty"`
	LocalAddr    string                    `json:"localAddr,omitempty"`
	RemoteAddr   string                    `json:"remoteAddr,omitempty"`
	Counters     fastpath.CountersSnapshot `json:"counters"`
	LastError    string                    `json:"lastError,omitempty"`
}

type ClientEnforcementEvent struct {
	At          string `json:"at"`
	ClientID    string `json:"clientId"`
	Reason      string `json:"reason"`
	ClosedPipes int    `json:"closedPipes"`
	Error       string `json:"error,omitempty"`
}

func NewRuntimeManager() *RuntimeManager {
	return NewRuntimeManagerWithFactory(func() RuntimeController {
		return core.NewSupervisor()
	})
}

func NewRuntimeManagerWithFactory(factory func() RuntimeController) *RuntimeManager {
	if factory == nil {
		factory = func() RuntimeController { return core.NewSupervisor() }
	}
	return &RuntimeManager{newController: factory}
}

func (m *RuntimeManager) Apply(runtime *config.GeneratedRuntime, cfg ...config.RuntimeConfig) (RuntimeState, error) {
	if runtime == nil {
		return m.State(), fmt.Errorf("runtime is nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.stopEnforcementLocked()
	if m.controller != nil && canPrepareRuntimeInParallel(m.activeRuntime, runtime) {
		return m.applyPreparedRuntimeLocked(runtime, cfg...)
	}
	if m.controller != nil {
		if err := m.controller.Stop(); err != nil {
			m.lastError = err.Error()
			return m.stateLocked(), err
		}
		m.controller = nil
		m.stoppedAt = time.Now()
	}

	next := m.newController()
	if err := next.Start(runtime); err != nil {
		_ = next.Stop()
		return m.rollbackAfterFailedApplyLocked(err)
	}

	now := time.Now()
	m.controller = next
	m.activeRuntime = cloneGeneratedRuntime(runtime)
	m.generation++
	m.startedAt = now
	m.lastAppliedAt = now
	m.stoppedAt = time.Time{}
	m.lastRollbackError = ""
	m.lastReloadMode = "stop-first"
	m.lastError = ""
	m.enforcementEvents = nil
	m.startAppliedEnforcementLocked(cfg...)
	return m.stateLocked(), nil
}

func (m *RuntimeManager) Stop() (RuntimeState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.stopEnforcementLocked()
	if m.controller == nil {
		return m.stateLocked(), nil
	}
	if err := m.controller.Stop(); err != nil {
		m.lastError = err.Error()
		return m.stateLocked(), err
	}
	m.controller = nil
	m.activeRuntime = nil
	m.activeConfig = nil
	m.stoppedAt = time.Now()
	return m.stateLocked(), nil
}

func (m *RuntimeManager) State() RuntimeState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stateLocked()
}

func (m *RuntimeManager) EnforceClientLimits(cfg config.RuntimeConfig, now time.Time) (RuntimeState, []ClientEnforcementEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	events, err := m.enforceClientLimitsLocked(cfg, now)
	return m.stateLocked(), events, err
}

func (m *RuntimeManager) stateLocked() RuntimeState {
	state := RuntimeState{
		Running:           m.controller != nil,
		Generation:        m.generation,
		LastError:         m.lastError,
		EnforcementEvents: append([]ClientEnforcementEvent(nil), m.enforcementEvents...),
	}
	if !m.startedAt.IsZero() {
		state.StartedAt = m.startedAt.UTC().Format(time.RFC3339Nano)
	}
	if !m.stoppedAt.IsZero() {
		state.StoppedAt = m.stoppedAt.UTC().Format(time.RFC3339Nano)
	}
	if !m.lastAppliedAt.IsZero() {
		state.LastAppliedAt = m.lastAppliedAt.UTC().Format(time.RFC3339Nano)
	}
	if !m.lastRollbackAt.IsZero() {
		state.LastRollbackAt = m.lastRollbackAt.UTC().Format(time.RFC3339Nano)
	}
	if !m.lastEnforcedAt.IsZero() {
		state.LastEnforcedAt = m.lastEnforcedAt.UTC().Format(time.RFC3339Nano)
	}
	if m.lastRollbackError != "" {
		state.LastRollbackError = m.lastRollbackError
	}
	if m.lastReloadMode != "" {
		state.LastReloadMode = m.lastReloadMode
	}
	if m.controller == nil {
		return state
	}
	for _, pipe := range m.controller.UDPPipes() {
		lastErr := ""
		if err := pipe.Err(); err != nil {
			lastErr = err.Error()
		}
		remote := pipe.RemoteAddr
		if accepted := pipe.AcceptedRemoteAddr(); accepted.IsValid() {
			remote = accepted
		}
		state.UDPPipes = append(state.UDPPipes, RuntimePipeState{
			EndpointID:   pipe.Pipe.EndpointID,
			EndpointKind: pipe.Pipe.EndpointKind,
			Transport:    "udp",
			RouteID:      pipe.Pipe.RouteID,
			DeviceID:     pipe.Pipe.DeviceID,
			DeviceName:   pipe.DeviceName,
			ClientID:     pipe.Pipe.Binding.ClientID,
			AddressID:    pipe.Pipe.Binding.AddressID,
			LocalAddr:    addrString(pipe.LocalAddr),
			RemoteAddr:   addrString(remote),
			LastError:    lastErr,
			Counters:     pipe.Counters(),
		})
	}
	for _, pipe := range m.controller.TCPPipes() {
		lastErr := ""
		if err := pipe.Err(); err != nil {
			lastErr = err.Error()
		}
		remote := pipe.RemoteAddr
		if accepted := pipe.AcceptedRemoteAddr(); accepted.IsValid() {
			remote = accepted
		}
		state.TCPPipes = append(state.TCPPipes, RuntimePipeState{
			EndpointID:   pipe.Pipe.EndpointID,
			EndpointKind: pipe.Pipe.EndpointKind,
			Transport:    "tcp",
			RouteID:      pipe.Pipe.RouteID,
			DeviceID:     pipe.Pipe.DeviceID,
			DeviceName:   pipe.DeviceName,
			ClientID:     pipe.Pipe.Binding.ClientID,
			AddressID:    pipe.Pipe.Binding.AddressID,
			LocalAddr:    addrString(pipe.LocalAddr),
			RemoteAddr:   addrString(remote),
			Counters:     pipe.Counters(),
			LastError:    lastErr,
		})
	}
	for _, pipe := range m.controller.XrayPipes() {
		lastErr := ""
		if err := pipe.Err(); err != nil {
			lastErr = err.Error()
		}
		state.XrayPipes = append(state.XrayPipes, RuntimePipeState{
			EndpointID:   pipe.Pipe.EndpointID,
			EndpointKind: pipe.Pipe.EndpointKind,
			Transport:    "xray",
			RouteID:      pipe.Pipe.RouteID,
			DeviceID:     pipe.Pipe.DeviceID,
			DeviceName:   pipe.DeviceName,
			ClientID:     pipe.Pipe.Binding.ClientID,
			AddressID:    pipe.Pipe.Binding.AddressID,
			RemoteAddr:   xrayRemoteAddr(pipe.Pipe),
			Counters:     pipe.Counters(),
			LastError:    lastErr,
		})
	}
	state.XrayRuntimes = append(state.XrayRuntimes, m.controller.XrayStates()...)
	return state
}

func (m *RuntimeManager) applyPreparedRuntimeLocked(runtime *config.GeneratedRuntime, cfg ...config.RuntimeConfig) (RuntimeState, error) {
	old := m.controller
	next := m.newController()
	if err := next.Start(runtime); err != nil {
		_ = next.Stop()
		m.lastError = fmt.Sprintf("%s; kept generation %d running", err.Error(), m.generation)
		m.lastReloadMode = "prepare-first-failed"
		m.restartActiveEnforcementLocked()
		return m.stateLocked(), err
	}
	if err := old.Stop(); err != nil {
		_ = next.Stop()
		m.lastError = err.Error()
		m.lastReloadMode = "prepare-first-stop-old-failed"
		m.restartActiveEnforcementLocked()
		return m.stateLocked(), err
	}

	now := time.Now()
	m.controller = next
	m.activeRuntime = cloneGeneratedRuntime(runtime)
	m.generation++
	m.startedAt = now
	m.lastAppliedAt = now
	m.stoppedAt = time.Time{}
	m.lastRollbackError = ""
	m.lastReloadMode = "prepare-first"
	m.lastError = ""
	m.enforcementEvents = nil
	m.startAppliedEnforcementLocked(cfg...)
	return m.stateLocked(), nil
}

func (m *RuntimeManager) rollbackAfterFailedApplyLocked(applyErr error) (RuntimeState, error) {
	oldRuntime := cloneGeneratedRuntime(m.activeRuntime)
	if oldRuntime == nil {
		m.lastError = applyErr.Error()
		m.startedAt = time.Time{}
		return m.stateLocked(), applyErr
	}

	rollback := m.newController()
	if err := rollback.Start(oldRuntime); err != nil {
		_ = rollback.Stop()
		now := time.Now()
		m.controller = nil
		m.activeRuntime = nil
		m.activeConfig = nil
		m.startedAt = time.Time{}
		m.stoppedAt = now
		m.lastRollbackAt = now
		m.lastRollbackError = err.Error()
		m.lastError = fmt.Sprintf("%s; rollback failed: %s", applyErr.Error(), err.Error())
		return m.stateLocked(), fmt.Errorf("runtime apply failed: %v; rollback failed: %w", applyErr, err)
	}

	now := time.Now()
	m.controller = rollback
	m.activeRuntime = oldRuntime
	m.startedAt = now
	m.stoppedAt = time.Time{}
	m.lastRollbackAt = now
	m.lastRollbackError = ""
	m.lastError = fmt.Sprintf("%s; rolled back to generation %d", applyErr.Error(), m.generation)
	m.restartActiveEnforcementLocked()
	return m.stateLocked(), fmt.Errorf("runtime apply failed: %v; rolled back to generation %d", applyErr, m.generation)
}

func (m *RuntimeManager) startAppliedEnforcementLocked(cfg ...config.RuntimeConfig) {
	if len(cfg) == 0 {
		m.activeConfig = nil
		return
	}
	cloned := cloneRuntimeConfigForEnforcement(cfg[0])
	m.activeConfig = &cloned
	m.startEnforcementLocked(cloned)
}

func (m *RuntimeManager) restartActiveEnforcementLocked() {
	if m.activeConfig == nil {
		return
	}
	m.startEnforcementLocked(cloneRuntimeConfigForEnforcement(*m.activeConfig))
}

func (m *RuntimeManager) startEnforcementLocked(cfg config.RuntimeConfig) {
	interval := enforcementInterval(cfg)
	stop := make(chan struct{})
	done := make(chan struct{})
	m.enforcementStop = stop
	m.enforcementDone = done
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_, _, _ = m.EnforceClientLimits(cfg, time.Now())
			case <-stop:
				return
			}
		}
	}()
	_, _ = m.enforceClientLimitsLocked(cfg, time.Now())
}

func (m *RuntimeManager) stopEnforcementLocked() {
	if m.enforcementStop == nil {
		return
	}
	close(m.enforcementStop)
	done := m.enforcementDone
	m.enforcementStop = nil
	m.enforcementDone = nil
	m.mu.Unlock()
	<-done
	m.mu.Lock()
}

func (m *RuntimeManager) enforceClientLimitsLocked(cfg config.RuntimeConfig, now time.Time) ([]ClientEnforcementEvent, error) {
	if m.controller == nil {
		return nil, nil
	}
	plans := BuildClientEnforcementPlan(cfg, m.stateLocked(), now)
	if len(plans) == 0 {
		return nil, nil
	}
	events := make([]ClientEnforcementEvent, 0, len(plans))
	var firstErr error
	at := now.UTC().Format(time.RFC3339Nano)
	for _, plan := range plans {
		closed, err := m.controller.CloseClientPipes(plan.ClientID)
		event := ClientEnforcementEvent{
			At:          at,
			ClientID:    plan.ClientID,
			Reason:      plan.Reason,
			ClosedPipes: closed,
		}
		if err != nil {
			event.Error = err.Error()
			if firstErr == nil {
				firstErr = err
			}
		}
		if closed > 0 || err != nil {
			events = append(events, event)
		}
	}
	if len(events) > 0 {
		m.lastEnforcedAt = now
		m.enforcementEvents = append(m.enforcementEvents, events...)
		if len(m.enforcementEvents) > 50 {
			m.enforcementEvents = append([]ClientEnforcementEvent(nil), m.enforcementEvents[len(m.enforcementEvents)-50:]...)
		}
	}
	if firstErr != nil {
		m.lastError = firstErr.Error()
	}
	return events, firstErr
}

func enforcementInterval(cfg config.RuntimeConfig) time.Duration {
	for _, item := range cfg.Settings {
		if item.Enabled && item.StatsIntervalSecond > 0 {
			return time.Duration(item.StatsIntervalSecond) * time.Second
		}
	}
	return 5 * time.Second
}

func cloneGeneratedRuntime(in *config.GeneratedRuntime) *config.GeneratedRuntime {
	if in == nil {
		return nil
	}
	out := &config.GeneratedRuntime{
		Devices:      cloneRuntimeDevices(in.Devices),
		Listeners:    append([]config.RuntimeEndpoint(nil), in.Listeners...),
		Connectors:   append([]config.RuntimeEndpoint(nil), in.Connectors...),
		Routes:       append([]config.RuntimeRoute(nil), in.Routes...),
		XrayProfiles: append([]config.RuntimeXrayProfile(nil), in.XrayProfiles...),
		Settings:     append([]config.RuntimeSettings(nil), in.Settings...),
		UDPPipes:     append([]config.RuntimeUDPPipe(nil), in.UDPPipes...),
		TCPPipes:     append([]config.RuntimeTCPPipe(nil), in.TCPPipes...),
		XrayPipes:    append([]config.RuntimeXrayPipe(nil), in.XrayPipes...),
	}
	for i := range out.UDPPipes {
		out.UDPPipes[i].AddressGuard = cloneRuntimeAddressGuard(in.UDPPipes[i].AddressGuard)
	}
	for i := range out.TCPPipes {
		out.TCPPipes[i].AddressGuard = cloneRuntimeAddressGuard(in.TCPPipes[i].AddressGuard)
	}
	for i := range out.XrayPipes {
		out.XrayPipes[i].AddressGuard = cloneRuntimeAddressGuard(in.XrayPipes[i].AddressGuard)
	}
	return out
}

func cloneRuntimeDevices(in []config.RuntimeDevice) []config.RuntimeDevice {
	out := append([]config.RuntimeDevice(nil), in...)
	for i := range out {
		out[i].Routes = append([]config.RuntimeDeviceRoute(nil), in[i].Routes...)
		out[i].DNS.Nameservers = append([]string(nil), in[i].DNS.Nameservers...)
		out[i].DNS.SearchDomains = append([]string(nil), in[i].DNS.SearchDomains...)
		out[i].DNS.Options = append([]string(nil), in[i].DNS.Options...)
	}
	return out
}

func cloneRuntimeAddressGuard(in config.RuntimeAddressGuard) config.RuntimeAddressGuard {
	return config.RuntimeAddressGuard{
		IPv4CIDRs: append([]string(nil), in.IPv4CIDRs...),
		IPv6CIDRs: append([]string(nil), in.IPv6CIDRs...),
		MACs:      append([]string(nil), in.MACs...),
	}
}

func addrString(value interface {
	IsValid() bool
	String() string
}) string {
	if !value.IsValid() {
		return ""
	}
	return value.String()
}

func xrayRemoteAddr(pipe config.RuntimeXrayPipe) string {
	if pipe.Remote == "" || pipe.Port == 0 {
		return ""
	}
	return fmt.Sprintf("%s:%d", pipe.Remote, pipe.Port)
}

func canPrepareRuntimeInParallel(oldRuntime, nextRuntime *config.GeneratedRuntime) bool {
	oldResources := collectPreparedRuntimeResources(oldRuntime)
	nextResources := collectPreparedRuntimeResources(nextRuntime)
	if len(oldResources) == 0 || len(nextResources) == 0 {
		return false
	}
	for key := range nextResources {
		if _, ok := oldResources[key]; ok {
			return false
		}
	}
	return true
}

func collectPreparedRuntimeResources(runtime *config.GeneratedRuntime) map[string]struct{} {
	out := map[string]struct{}{}
	if runtime == nil {
		return out
	}
	for _, device := range runtime.Devices {
		addRuntimeResource(out, "if", device.IfName)
		if device.Bridge.Enabled {
			addRuntimeResource(out, "bridge", device.Bridge.Name)
			addRuntimeResource(out, "bridge-member", device.Bridge.IfName)
		}
	}
	for _, endpoint := range runtime.Listeners {
		if endpoint.BindPort == 0 {
			continue
		}
		out[fmt.Sprintf("listen/%s/%d", listenerProtocol(endpoint.Transport), endpoint.BindPort)] = struct{}{}
	}
	return out
}

func addRuntimeResource(resources map[string]struct{}, prefix, value string) {
	if value == "" {
		return
	}
	resources[prefix+"/"+value] = struct{}{}
}

func listenerProtocol(transport model.Transport) string {
	if transport == model.TransportUDP {
		return "udp"
	}
	return "tcp"
}

func cloneRuntimeConfigForEnforcement(in config.RuntimeConfig) config.RuntimeConfig {
	return config.RuntimeConfig{
		Devices:  append([]model.Device(nil), in.Devices...),
		Clients:  append([]model.Client(nil), in.Clients...),
		Routes:   append([]model.Route(nil), in.Routes...),
		Settings: append([]model.Settings(nil), in.Settings...),
	}
}
