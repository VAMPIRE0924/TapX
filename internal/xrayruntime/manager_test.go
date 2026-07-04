package xrayruntime

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"tapx/internal/config"
	"tapx/internal/model"
)

func TestManagerStartsExternalRuntimeAndCleansConfig(t *testing.T) {
	dataDir := t.TempDir()
	var seenConfigPath string
	manager := NewManagerWithCommandFactory(func(binaryPath, configPath string) *exec.Cmd {
		if binaryPath != "fake-xray" {
			t.Fatalf("binary path = %q, want fake-xray", binaryPath)
		}
		seenConfigPath = configPath
		cmd := exec.Command(os.Args[0], "-test.run=TestXrayRuntimeHelperProcess", "--", configPath)
		cmd.Env = append(os.Environ(), "TAPX_XRAY_HELPER=1")
		return cmd
	})

	runtime := externalRuntimeForTest(dataDir)
	if err := manager.Start(runtime); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	state := manager.State()
	if !state.Running || state.PID == 0 || state.EndpointCount != 1 || state.ConfigPath == "" {
		t.Fatalf("state after start = %+v, want running xray state", state)
	}
	if seenConfigPath == "" {
		t.Fatal("test command factory did not see config path")
	}
	payload, err := os.ReadFile(seenConfigPath)
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(payload, &doc); err != nil {
		t.Fatalf("generated config is invalid JSON: %v", err)
	}
	if err := manager.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if _, err := os.Stat(seenConfigPath); !os.IsNotExist(err) {
		t.Fatalf("generated config still exists or stat failed: %v", err)
	}
}

func TestManagerStartsEmbeddedRuntimePrototype(t *testing.T) {
	manager := NewManagerWithAdapters(func(binaryPath, configPath string) *exec.Cmd {
		t.Fatal("command factory should not be called for embedded runtime")
		return nil
	}, NewPrototypeEmbeddedAdapter())
	runtime := embeddedRuntimeForTest(t)
	if err := manager.Start(runtime); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	state := manager.State()
	if !state.Running || state.Runtime != "embedded" || state.Adapter != "prototype" || state.EndpointCount != 1 {
		t.Fatalf("state after start = %+v, want embedded prototype state", state)
	}
	if state.PID != 0 || state.ConfigPath != "" {
		t.Fatalf("state after start = %+v, want no external process", state)
	}
	if err := manager.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	stopped := manager.State()
	if stopped.Running || stopped.EndpointCount != 0 {
		t.Fatalf("state after stop = %+v, want stopped", stopped)
	}
}

func TestManagerStartsEmbeddedXrayCoreRuntime(t *testing.T) {
	manager := NewManagerWithCommandFactory(func(binaryPath, configPath string) *exec.Cmd {
		t.Fatal("command factory should not be called for embedded runtime")
		return nil
	})
	runtime := embeddedRuntimeForTest(t)
	if err := manager.Start(runtime); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	state := manager.State()
	if !state.Running || state.Runtime != "embedded" || state.Adapter != "xray-core" || state.EndpointCount != 1 {
		t.Fatalf("state after start = %+v, want embedded xray-core state", state)
	}
	if state.PID != 0 || state.ConfigPath != "" {
		t.Fatalf("state after start = %+v, want no external process", state)
	}
	if err := manager.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	stopped := manager.State()
	if stopped.Running || stopped.EndpointCount != 0 {
		t.Fatalf("state after stop = %+v, want stopped", stopped)
	}
}

func TestManagerDialEmbeddedTCPPassesOutboundTag(t *testing.T) {
	adapter := &recordingDialAdapter{}
	manager := NewManagerWithAdapters(func(binaryPath, configPath string) *exec.Cmd {
		t.Fatal("command factory should not be called for embedded runtime")
		return nil
	}, adapter)
	runtime := embeddedRuntimeForTest(t)
	if err := manager.Start(runtime); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	conn, err := manager.DialEmbeddedTCP(context.Background(), "connector-xr", "tapx.frame.local", 1)
	if err != nil {
		t.Fatalf("DialEmbeddedTCP() error = %v", err)
	}
	_ = conn.Close()
	if adapter.outboundTag != "connector-xr" || adapter.host != "tapx.frame.local" || adapter.port != 1 {
		t.Fatalf("dial args = tag:%q host:%q port:%d, want connector-xr tapx.frame.local 1", adapter.outboundTag, adapter.host, adapter.port)
	}
}

func TestXrayRuntimeHelperProcess(t *testing.T) {
	if os.Getenv("TAPX_XRAY_HELPER") != "1" {
		return
	}
	if len(os.Args) == 0 {
		os.Exit(2)
	}
	configPath := os.Args[len(os.Args)-1]
	if filepath.Base(configPath) == "" {
		os.Exit(2)
	}
	if _, err := os.ReadFile(configPath); err != nil {
		os.Exit(3)
	}
	time.Sleep(30 * time.Second)
	os.Exit(0)
}

type recordingDialAdapter struct {
	endpoints   []EndpointRef
	outboundTag string
	host        string
	port        uint16
}

func (a *recordingDialAdapter) Start(cfg EmbeddedConfig) error {
	a.endpoints = append([]EndpointRef(nil), cfg.Endpoints...)
	return nil
}

func (a *recordingDialAdapter) Stop() error {
	a.endpoints = nil
	return nil
}

func (a *recordingDialAdapter) State() EmbeddedState {
	return EmbeddedState{
		Running:   len(a.endpoints) > 0,
		Adapter:   "recording",
		Endpoints: append([]EndpointRef(nil), a.endpoints...),
	}
}

func (a *recordingDialAdapter) DialTCP(_ context.Context, outboundTag string, host string, port uint16) (net.Conn, error) {
	a.outboundTag = outboundTag
	a.host = host
	a.port = port
	left, right := net.Pipe()
	go func() {
		_, _ = io.Copy(io.Discard, right)
		_ = right.Close()
	}()
	return left, nil
}

func embeddedRuntimeForTest(t *testing.T) *config.GeneratedRuntime {
	t.Helper()
	port := freeTCPPort(t)
	return &config.GeneratedRuntime{
		XrayProfiles: []config.RuntimeXrayProfile{{
			ID:                  "xr1",
			Runtime:             model.XrayEmbedded,
			InboundProtocol:     "dokodemo-door",
			InboundSettingsJSON: `{"address":"127.0.0.1","port":80,"network":"tcp"}`,
			AdvancedJSON:        `{"outbounds":[{"tag":"direct","protocol":"freedom"}],"routing":{"rules":[{"type":"field","inboundTag":["listener-xr"],"outboundTag":"direct"}]}}`,
		}},
		Listeners: []config.RuntimeEndpoint{{
			ID:            "listener-xr",
			Transport:     model.TransportXray,
			BindHost:      "127.0.0.1",
			BindPort:      uint16(port),
			XrayProfileID: "xr1",
		}},
	}
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen free tcp port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func externalRuntimeForTest(dataDir string) *config.GeneratedRuntime {
	return &config.GeneratedRuntime{
		Settings: []config.RuntimeSettings{{
			ExternalXrayPath: "fake-xray",
			DataDir:          dataDir,
		}},
		XrayProfiles: []config.RuntimeXrayProfile{{
			ID:                  "xr1",
			Runtime:             model.XrayExternal,
			InboundProtocol:     "dokodemo-door",
			InboundSettingsJSON: `{"address":"127.0.0.1","port":80,"network":"tcp"}`,
		}},
		Listeners: []config.RuntimeEndpoint{{
			ID:            "listener-xr",
			Transport:     model.TransportXray,
			BindHost:      "127.0.0.1",
			BindPort:      18080,
			XrayProfileID: "xr1",
		}},
	}
}
