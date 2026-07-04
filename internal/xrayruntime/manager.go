package xrayruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"tapx/internal/config"
)

type CommandFactory func(binaryPath, configPath string) *exec.Cmd

type Manager struct {
	mu              sync.Mutex
	commandFactory  CommandFactory
	embeddedAdapter EmbeddedAdapter
	cmd             *exec.Cmd
	done            chan error
	configPath      string
	tempDir         string
	startedAt       time.Time
	exitedAt        time.Time
	lastError       string
	externalRefs    []EndpointRef
	embeddedRefs    []EndpointRef
	streamHandlers  map[string]StreamHandler
}

type State struct {
	Running       bool          `json:"running"`
	Runtime       string        `json:"runtime,omitempty"`
	Adapter       string        `json:"adapter,omitempty"`
	PID           int           `json:"pid,omitempty"`
	ConfigPath    string        `json:"configPath,omitempty"`
	StartedAt     string        `json:"startedAt,omitempty"`
	ExitedAt      string        `json:"exitedAt,omitempty"`
	LastError     string        `json:"lastError,omitempty"`
	EndpointCount int           `json:"endpointCount"`
	Endpoints     []EndpointRef `json:"endpoints,omitempty"`
}

func NewManager() *Manager {
	return NewManagerWithCommandFactory(defaultCommandFactory)
}

func NewManagerWithCommandFactory(factory CommandFactory) *Manager {
	if factory == nil {
		factory = defaultCommandFactory
	}
	return NewManagerWithAdapters(factory, NewXrayCoreEmbeddedAdapter())
}

func NewManagerWithAdapters(factory CommandFactory, embedded EmbeddedAdapter) *Manager {
	if factory == nil {
		factory = defaultCommandFactory
	}
	if embedded == nil {
		embedded = NewPrototypeEmbeddedAdapter()
	}
	return &Manager{commandFactory: factory, embeddedAdapter: embedded}
}

func (m *Manager) RegisterStreamHandler(tag string, handler StreamHandler) error {
	if strings.TrimSpace(tag) == "" {
		return fmt.Errorf("xray: stream handler tag is required")
	}
	if handler == nil {
		return fmt.Errorf("xray: stream handler %s is nil", tag)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd != nil || len(m.embeddedRefs) > 0 {
		return fmt.Errorf("xray: cannot register stream handler %s after start", tag)
	}
	if m.streamHandlers == nil {
		m.streamHandlers = make(map[string]StreamHandler)
	}
	if _, ok := m.streamHandlers[tag]; ok {
		return fmt.Errorf("xray: duplicate stream handler %s", tag)
	}
	m.streamHandlers[tag] = handler
	return nil
}

func (m *Manager) DialEmbeddedTCP(ctx context.Context, outboundTag string, host string, port uint16) (net.Conn, error) {
	m.mu.Lock()
	adapter := m.embeddedAdapter
	running := len(m.embeddedRefs) > 0
	m.mu.Unlock()
	if !running {
		return nil, fmt.Errorf("xray: embedded runtime is not running")
	}
	dialer, ok := adapter.(EmbeddedDialer)
	if !ok {
		return nil, fmt.Errorf("xray: embedded adapter does not support dial")
	}
	return dialer.DialTCP(ctx, outboundTag, host, port)
}

func (m *Manager) Start(runtime *config.GeneratedRuntime) error {
	compiled, err := Compile(runtime)
	if err != nil {
		return err
	}
	if len(compiled.ExternalEndpoints) == 0 && len(compiled.EmbeddedEndpoints) == 0 {
		return nil
	}

	m.mu.Lock()
	if m.cmd != nil || len(m.embeddedRefs) > 0 {
		m.mu.Unlock()
		return fmt.Errorf("xray: manager already started")
	}
	m.mu.Unlock()

	if len(compiled.EmbeddedEndpoints) > 0 {
		m.mu.Lock()
		handlers := cloneStreamHandlers(m.streamHandlers)
		m.mu.Unlock()
		if err := m.embeddedAdapter.Start(EmbeddedConfig{
			Endpoints:      compiled.EmbeddedEndpoints,
			Document:       compiled.EmbeddedDocument,
			StreamHandlers: handlers,
		}); err != nil {
			return err
		}
	}

	var cmd *exec.Cmd
	var done chan error
	var configPath string
	var tempDir string
	if len(compiled.ExternalEndpoints) > 0 {
		cmd, done, configPath, tempDir, err = m.startExternal(runtime, compiled.Document)
		if err != nil {
			_ = m.embeddedAdapter.Stop()
			return err
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd != nil || len(m.embeddedRefs) > 0 {
		if cmd != nil {
			_ = stopProcess(cmd, done)
			cleanupConfig(configPath, tempDir)
		}
		_ = m.embeddedAdapter.Stop()
		return fmt.Errorf("xray: manager already started")
	}
	m.cmd = cmd
	m.done = done
	m.configPath = configPath
	m.tempDir = tempDir
	m.startedAt = time.Now()
	m.exitedAt = time.Time{}
	m.lastError = ""
	m.externalRefs = append([]EndpointRef(nil), compiled.ExternalEndpoints...)
	m.embeddedRefs = append([]EndpointRef(nil), compiled.EmbeddedEndpoints...)
	return nil
}

func (m *Manager) startExternal(runtime *config.GeneratedRuntime, document map[string]any) (*exec.Cmd, chan error, string, string, error) {
	settings := firstSettings(runtime.Settings)
	binaryPath := strings.TrimSpace(settings.ExternalXrayPath)
	if binaryPath == "" {
		return nil, nil, "", "", fmt.Errorf("xray: Settings.ExternalXrayPath is required for external runtime")
	}
	configPath, tempDir, err := writeConfig(settings.DataDir, document)
	if err != nil {
		return nil, nil, "", "", err
	}

	cmd := m.commandFactory(binaryPath, configPath)
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Start(); err != nil {
		cleanupConfig(configPath, tempDir)
		return nil, nil, "", "", fmt.Errorf("xray: start external runtime: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		cleanupConfig(configPath, tempDir)
		if err == nil {
			return nil, nil, "", "", fmt.Errorf("xray: external runtime exited during startup")
		}
		return nil, nil, "", "", fmt.Errorf("xray: external runtime exited during startup: %w", err)
	case <-time.After(150 * time.Millisecond):
	}
	return cmd, done, configPath, tempDir, nil
}

func (m *Manager) Stop() error {
	m.mu.Lock()
	cmd := m.cmd
	done := m.done
	configPath := m.configPath
	tempDir := m.tempDir
	hasEmbedded := len(m.embeddedRefs) > 0
	if cmd == nil && !hasEmbedded {
		m.mu.Unlock()
		return nil
	}
	m.cmd = nil
	m.done = nil
	m.configPath = ""
	m.tempDir = ""
	m.externalRefs = nil
	m.embeddedRefs = nil
	m.mu.Unlock()

	var err error
	if cmd != nil {
		err = stopProcess(cmd, done)
	}
	cleanupConfig(configPath, tempDir)
	if embeddedErr := m.embeddedAdapter.Stop(); embeddedErr != nil && err == nil {
		err = embeddedErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.exitedAt = time.Now()
	if err != nil {
		m.lastError = err.Error()
	}
	return err
}

func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refreshLocked()
	return m.stateLocked()
}

func (m *Manager) refreshLocked() {
	if m.cmd == nil || m.done == nil {
		return
	}
	select {
	case err := <-m.done:
		m.cmd = nil
		m.done = nil
		m.exitedAt = time.Now()
		if err != nil {
			m.lastError = err.Error()
		}
	default:
	}
}

func (m *Manager) stateLocked() State {
	embeddedState := m.embeddedAdapter.State()
	endpoints := append([]EndpointRef(nil), m.externalRefs...)
	endpoints = append(endpoints, embeddedState.Endpoints...)
	state := State{
		Running:       m.cmd != nil || embeddedState.Running,
		Runtime:       runtimeLabel(m.cmd != nil, embeddedState.Running),
		Adapter:       adapterLabel(m.cmd != nil, embeddedState),
		ConfigPath:    m.configPath,
		LastError:     first(m.lastError, embeddedState.LastError),
		EndpointCount: len(endpoints),
		Endpoints:     endpoints,
	}
	if m.cmd != nil && m.cmd.Process != nil {
		state.PID = m.cmd.Process.Pid
	}
	if !m.startedAt.IsZero() {
		state.StartedAt = m.startedAt.UTC().Format(time.RFC3339Nano)
	}
	if !m.exitedAt.IsZero() {
		state.ExitedAt = m.exitedAt.UTC().Format(time.RFC3339Nano)
	}
	return state
}

func runtimeLabel(externalRunning bool, embeddedRunning bool) string {
	switch {
	case externalRunning && embeddedRunning:
		return "mixed"
	case externalRunning:
		return "external"
	case embeddedRunning:
		return "embedded"
	default:
		return ""
	}
}

func adapterLabel(externalRunning bool, embedded EmbeddedState) string {
	if externalRunning && embedded.Running {
		return "external+" + embedded.Adapter
	}
	if embedded.Running {
		return embedded.Adapter
	}
	return ""
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func defaultCommandFactory(binaryPath, configPath string) *exec.Cmd {
	return exec.Command(binaryPath, "run", "-config", configPath)
}

func stopProcess(cmd *exec.Cmd, done <-chan error) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		if killErr := cmd.Process.Kill(); killErr != nil {
			return killErr
		}
		<-done
		return nil
	}
	select {
	case <-done:
		return nil
	case <-time.After(5 * time.Second):
		if err := cmd.Process.Kill(); err != nil {
			return err
		}
		<-done
		return nil
	}
}

func writeConfig(dataDir string, document map[string]any) (string, string, error) {
	var dir string
	var tempDir string
	var err error
	if strings.TrimSpace(dataDir) == "" {
		tempDir, err = os.MkdirTemp("", "tapx-xray-")
		if err != nil {
			return "", "", fmt.Errorf("xray: create temp dir: %w", err)
		}
		dir = tempDir
	} else {
		dir = filepath.Join(dataDir, "xray")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", "", fmt.Errorf("xray: create config dir: %w", err)
		}
	}

	payload, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		cleanupConfig("", tempDir)
		return "", "", fmt.Errorf("xray: marshal config: %w", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("tapx-xray-%d.json", time.Now().UnixNano()))
	if err := os.WriteFile(path, append(payload, '\n'), 0o600); err != nil {
		cleanupConfig("", tempDir)
		return "", "", fmt.Errorf("xray: write config: %w", err)
	}
	return path, tempDir, nil
}

func cleanupConfig(path, tempDir string) {
	if path != "" {
		_ = os.Remove(path)
	}
	if tempDir != "" {
		_ = os.RemoveAll(tempDir)
	}
}

func firstSettings(settings []config.RuntimeSettings) config.RuntimeSettings {
	for _, item := range settings {
		return item
	}
	return config.RuntimeSettings{}
}

func endpointList(endpoints []EndpointRef) string {
	parts := make([]string, 0, len(endpoints))
	for _, item := range endpoints {
		parts = append(parts, item.Kind+"/"+item.ID)
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func cloneStreamHandlers(in map[string]StreamHandler) map[string]StreamHandler {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]StreamHandler, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
