package panel

import (
	"errors"
	"strings"
	"testing"
	"time"

	"tapx/internal/config"
	"tapx/internal/core"
	"tapx/internal/model"
	"tapx/internal/xrayruntime"
)

func TestRuntimeManagerApplyStateAndStop(t *testing.T) {
	manager := NewRuntimeManager()

	state, err := manager.Apply(&config.GeneratedRuntime{})
	if err != nil {
		t.Fatalf("apply empty runtime: %v", err)
	}
	if !state.Running {
		t.Fatalf("expected running state after apply: %+v", state)
	}
	if state.Generation != 1 {
		t.Fatalf("generation = %d, want 1", state.Generation)
	}
	if state.StartedAt == "" || state.LastAppliedAt == "" {
		t.Fatalf("expected timestamps after apply: %+v", state)
	}

	state = manager.State()
	if !state.Running || state.Generation != 1 {
		t.Fatalf("unexpected state: %+v", state)
	}

	state, err = manager.Stop()
	if err != nil {
		t.Fatalf("stop runtime: %v", err)
	}
	if state.Running {
		t.Fatalf("expected stopped state: %+v", state)
	}
	if state.StoppedAt == "" {
		t.Fatalf("expected stopped timestamp: %+v", state)
	}
}

func TestRuntimeManagerRejectsNilRuntime(t *testing.T) {
	manager := NewRuntimeManager()
	if _, err := manager.Apply(nil); err == nil {
		t.Fatalf("expected nil runtime error")
	}
}

func TestRuntimeManagerRollsBackWhenReplacementStartFails(t *testing.T) {
	startErr := errors.New("bind failed")
	old := &fakeRuntimeController{}
	failed := &fakeRuntimeController{startErr: startErr}
	rollback := &fakeRuntimeController{}
	manager := newFakeRuntimeManager(old, failed, rollback)

	first := &config.GeneratedRuntime{
		Devices: []config.RuntimeDevice{{
			ID:     "tun-a",
			Routes: []config.RuntimeDeviceRoute{{Enabled: true, Destination: "10.10.0.0/24"}},
			DNS:    config.RuntimeDNS{Enabled: true, Nameservers: []string{"1.1.1.1"}},
		}},
		Settings: []config.RuntimeSettings{{ID: "old"}},
	}
	if _, err := manager.Apply(first); err != nil {
		t.Fatalf("apply first runtime: %v", err)
	}
	first.Settings[0].ID = "mutated-after-apply"
	first.Devices[0].Routes[0].Destination = "mutated"
	first.Devices[0].DNS.Nameservers[0] = "9.9.9.9"

	state, err := manager.Apply(&config.GeneratedRuntime{Settings: []config.RuntimeSettings{{ID: "new"}}})
	if err == nil {
		t.Fatalf("expected replacement apply error")
	}
	if !state.Running {
		t.Fatalf("expected old runtime to be running after rollback: %+v", state)
	}
	if state.Generation != 1 {
		t.Fatalf("generation = %d, want 1 after rollback", state.Generation)
	}
	if state.LastRollbackAt == "" {
		t.Fatalf("expected rollback timestamp: %+v", state)
	}
	if !strings.Contains(state.LastError, "rolled back to generation 1") {
		t.Fatalf("last error = %q, want rollback note", state.LastError)
	}
	if old.stopCalls != 1 {
		t.Fatalf("old stop calls = %d, want 1", old.stopCalls)
	}
	if failed.startCalls != 1 || failed.stopCalls != 1 {
		t.Fatalf("failed controller calls: start=%d stop=%d, want 1/1", failed.startCalls, failed.stopCalls)
	}
	if rollback.startCalls != 1 {
		t.Fatalf("rollback start calls = %d, want 1", rollback.startCalls)
	}
	if rollback.runtime == nil || len(rollback.runtime.Settings) != 1 || rollback.runtime.Settings[0].ID != "old" {
		t.Fatalf("rollback runtime = %+v, want cloned old runtime", rollback.runtime)
	}
	if got := rollback.runtime.Devices[0].Routes[0].Destination; got != "10.10.0.0/24" {
		t.Fatalf("rollback device route = %q, want cloned destination", got)
	}
	if got := rollback.runtime.Devices[0].DNS.Nameservers[0]; got != "1.1.1.1" {
		t.Fatalf("rollback DNS nameserver = %q, want cloned nameserver", got)
	}
}

func TestRuntimeManagerReportsRollbackFailure(t *testing.T) {
	old := &fakeRuntimeController{}
	failed := &fakeRuntimeController{startErr: errors.New("new runtime failed")}
	rollback := &fakeRuntimeController{startErr: errors.New("old runtime failed")}
	manager := newFakeRuntimeManager(old, failed, rollback)

	if _, err := manager.Apply(&config.GeneratedRuntime{}); err != nil {
		t.Fatalf("apply first runtime: %v", err)
	}

	state, err := manager.Apply(&config.GeneratedRuntime{})
	if err == nil {
		t.Fatalf("expected replacement and rollback error")
	}
	if state.Running {
		t.Fatalf("expected stopped state after rollback failure: %+v", state)
	}
	if state.LastRollbackAt == "" || state.LastRollbackError == "" {
		t.Fatalf("expected rollback failure fields: %+v", state)
	}
	if !strings.Contains(state.LastError, "rollback failed") {
		t.Fatalf("last error = %q, want rollback failure note", state.LastError)
	}
}

func TestRuntimeManagerPrepareFirstReloadForDisjointResources(t *testing.T) {
	events := []string{}
	old := &fakeRuntimeController{name: "old", events: &events}
	next := &fakeRuntimeController{name: "next", events: &events}
	manager := newFakeRuntimeManager(old, next)

	oldRuntime := &config.GeneratedRuntime{
		Devices:   []config.RuntimeDevice{{ID: "old-tun", IfName: "tapx-old"}},
		Listeners: []config.RuntimeEndpoint{{ID: "old-udp", Transport: model.TransportUDP, BindPort: 41001}},
	}
	nextRuntime := &config.GeneratedRuntime{
		Devices:   []config.RuntimeDevice{{ID: "next-tun", IfName: "tapx-next"}},
		Listeners: []config.RuntimeEndpoint{{ID: "next-udp", Transport: model.TransportUDP, BindPort: 41002}},
	}

	if _, err := manager.Apply(oldRuntime); err != nil {
		t.Fatalf("apply old runtime: %v", err)
	}
	state, err := manager.Apply(nextRuntime)
	if err != nil {
		t.Fatalf("apply next runtime: %v", err)
	}
	if state.Generation != 2 || state.LastReloadMode != "prepare-first" {
		t.Fatalf("state = %+v, want prepare-first generation 2", state)
	}
	if got := strings.Join(events, ","); got != "old.start,next.start,old.stop" {
		t.Fatalf("events = %s, want prepare-first order", got)
	}
}

func TestRuntimeManagerUsesStopFirstWhenResourcesConflict(t *testing.T) {
	events := []string{}
	old := &fakeRuntimeController{name: "old", events: &events}
	next := &fakeRuntimeController{name: "next", events: &events}
	manager := newFakeRuntimeManager(old, next)

	oldRuntime := &config.GeneratedRuntime{
		Devices:   []config.RuntimeDevice{{ID: "old-tun", IfName: "tapx0"}},
		Listeners: []config.RuntimeEndpoint{{ID: "old-udp", Transport: model.TransportUDP, BindPort: 41001}},
	}
	nextRuntime := &config.GeneratedRuntime{
		Devices:   []config.RuntimeDevice{{ID: "next-tun", IfName: "tapx0"}},
		Listeners: []config.RuntimeEndpoint{{ID: "next-udp", Transport: model.TransportUDP, BindPort: 41002}},
	}

	if _, err := manager.Apply(oldRuntime); err != nil {
		t.Fatalf("apply old runtime: %v", err)
	}
	state, err := manager.Apply(nextRuntime)
	if err != nil {
		t.Fatalf("apply next runtime: %v", err)
	}
	if state.Generation != 2 || state.LastReloadMode != "stop-first" {
		t.Fatalf("state = %+v, want stop-first generation 2", state)
	}
	if got := strings.Join(events, ","); got != "old.start,old.stop,next.start" {
		t.Fatalf("events = %s, want stop-first order", got)
	}
}

func TestRuntimeManagerPrepareFirstFailureKeepsOldRuntime(t *testing.T) {
	events := []string{}
	old := &fakeRuntimeController{name: "old", events: &events}
	failed := &fakeRuntimeController{name: "failed", events: &events, startErr: errors.New("new bind failed")}
	manager := newFakeRuntimeManager(old, failed)

	oldRuntime := &config.GeneratedRuntime{
		Devices:   []config.RuntimeDevice{{ID: "old-tun", IfName: "tapx-old"}},
		Listeners: []config.RuntimeEndpoint{{ID: "old-udp", Transport: model.TransportUDP, BindPort: 41001}},
	}
	nextRuntime := &config.GeneratedRuntime{
		Devices:   []config.RuntimeDevice{{ID: "next-tun", IfName: "tapx-next"}},
		Listeners: []config.RuntimeEndpoint{{ID: "next-udp", Transport: model.TransportUDP, BindPort: 41002}},
	}

	if _, err := manager.Apply(oldRuntime); err != nil {
		t.Fatalf("apply old runtime: %v", err)
	}
	state, err := manager.Apply(nextRuntime)
	if err == nil {
		t.Fatalf("expected prepare-first start failure")
	}
	if !state.Running || state.Generation != 1 || state.LastReloadMode != "prepare-first-failed" {
		t.Fatalf("state = %+v, want old runtime kept", state)
	}
	if old.stopCalls != 0 {
		t.Fatalf("old stop calls = %d, want 0", old.stopCalls)
	}
	if got := strings.Join(events, ","); got != "old.start,failed.start,failed.stop" {
		t.Fatalf("events = %s, want failed prepare order", got)
	}
}

func TestRuntimeManagerEnforcesExpiredClient(t *testing.T) {
	controller := &fakeRuntimeController{
		udpPipes: []*core.UDPPipeHandle{{
			Pipe: config.RuntimeUDPPipe{
				EndpointID: "udp-a",
				DeviceID:   "tun-a",
				Binding:    config.RuntimeBinding{ClientID: "client-a"},
			},
		}},
	}
	manager := newFakeRuntimeManager(controller)
	cfg := config.RuntimeConfig{
		Clients: []model.Client{{ID: "client-a", Enabled: true, ExpiresAt: 1000}},
	}
	if _, err := manager.Apply(&config.GeneratedRuntime{}); err != nil {
		t.Fatalf("apply runtime: %v", err)
	}

	state, events, err := manager.EnforceClientLimits(cfg, time.Unix(2000, 0))
	if err != nil {
		t.Fatalf("enforce client limits: %v", err)
	}
	if len(events) != 1 || events[0].ClientID != "client-a" || events[0].Reason != "expired" || events[0].ClosedPipes != 1 {
		t.Fatalf("events = %+v, want expired client close", events)
	}
	if len(controller.closeCalls) != 1 || controller.closeCalls[0] != "client-a" {
		t.Fatalf("close calls = %+v, want client-a", controller.closeCalls)
	}
	if state.LastEnforcedAt == "" || len(state.EnforcementEvents) != 1 {
		t.Fatalf("state enforcement fields = %+v", state)
	}
}

func TestRuntimeManagerReportsXrayPipeState(t *testing.T) {
	controller := &fakeRuntimeController{
		xrayPipes: []*core.XrayPipeHandle{{
			Pipe: config.RuntimeXrayPipe{
				EndpointID:   "xray-a",
				EndpointKind: "connector",
				RouteID:      "route-a",
				DeviceID:     "tun-a",
				Remote:       "example.com",
				Port:         443,
				Binding: config.RuntimeBinding{
					ClientID:  "client-a",
					AddressID: "addr-a",
				},
			},
			DeviceName: "tapxxray0",
		}},
	}
	manager := newFakeRuntimeManager(controller)

	state, err := manager.Apply(&config.GeneratedRuntime{})
	if err != nil {
		t.Fatalf("apply runtime: %v", err)
	}
	if len(state.XrayPipes) != 1 {
		t.Fatalf("xray pipes = %+v, want one", state.XrayPipes)
	}
	pipe := state.XrayPipes[0]
	if pipe.Transport != "xray" || pipe.DeviceName != "tapxxray0" || pipe.RemoteAddr != "example.com:443" {
		t.Fatalf("xray pipe state = %+v, want xray tapxxray0 example.com:443", pipe)
	}
	if pipe.ClientID != "client-a" || pipe.AddressID != "addr-a" || pipe.RouteID != "route-a" {
		t.Fatalf("xray pipe binding state = %+v, want route/client/address", pipe)
	}
}

func newFakeRuntimeManager(controllers ...*fakeRuntimeController) *RuntimeManager {
	next := 0
	return NewRuntimeManagerWithFactory(func() RuntimeController {
		if next >= len(controllers) {
			panic("unexpected runtime controller allocation")
		}
		controller := controllers[next]
		next++
		return controller
	})
}

type fakeRuntimeController struct {
	name       string
	events     *[]string
	startErr   error
	stopErr    error
	closeErr   error
	startCalls int
	stopCalls  int
	closeCalls []string
	runtime    *config.GeneratedRuntime
	udpPipes   []*core.UDPPipeHandle
	tcpPipes   []*core.TCPPipeHandle
	xrayPipes  []*core.XrayPipeHandle
}

func (c *fakeRuntimeController) Start(runtime *config.GeneratedRuntime) error {
	c.startCalls++
	c.addEvent("start")
	c.runtime = runtime
	return c.startErr
}

func (c *fakeRuntimeController) Stop() error {
	c.stopCalls++
	c.addEvent("stop")
	return c.stopErr
}

func (c *fakeRuntimeController) addEvent(action string) {
	if c.events == nil || c.name == "" {
		return
	}
	*c.events = append(*c.events, c.name+"."+action)
}

func (c *fakeRuntimeController) CloseClientPipes(clientID string) (int, error) {
	c.closeCalls = append(c.closeCalls, clientID)
	if c.closeErr != nil {
		return 0, c.closeErr
	}
	return 1, nil
}

func (c *fakeRuntimeController) UDPPipes() []*core.UDPPipeHandle {
	return c.udpPipes
}

func (c *fakeRuntimeController) TCPPipes() []*core.TCPPipeHandle {
	return c.tcpPipes
}

func (c *fakeRuntimeController) XrayPipes() []*core.XrayPipeHandle {
	return c.xrayPipes
}

func (c *fakeRuntimeController) XrayStates() []xrayruntime.State {
	return nil
}
