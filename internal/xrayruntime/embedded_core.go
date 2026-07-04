package xrayruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	xnet "github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/net/cnc"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/common/session"
	xraycore "github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/features/outbound"
	_ "github.com/xtls/xray-core/main/distro/all"
	"github.com/xtls/xray-core/transport"
)

type xrayCoreEmbeddedAdapter struct {
	mu        sync.Mutex
	instance  *xraycore.Instance
	startedAt time.Time
	exitedAt  time.Time
	lastError string
	endpoints []EndpointRef
}

func NewXrayCoreEmbeddedAdapter() EmbeddedAdapter {
	return &xrayCoreEmbeddedAdapter{}
}

func (a *xrayCoreEmbeddedAdapter) Start(cfg EmbeddedConfig) error {
	if len(cfg.Endpoints) == 0 {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.instance != nil {
		return fmt.Errorf("xray: embedded adapter already started")
	}
	if len(cfg.Document) == 0 {
		a.lastError = "embedded xray document is empty"
		return fmt.Errorf("xray: embedded document is empty")
	}
	payload, err := json.Marshal(cfg.Document)
	if err != nil {
		a.lastError = err.Error()
		return fmt.Errorf("xray: marshal embedded document: %w", err)
	}
	coreConfig, err := xraycore.LoadConfig("json", bytes.NewReader(payload))
	if err != nil {
		a.lastError = err.Error()
		return fmt.Errorf("xray: load embedded config: %w", err)
	}
	instance, err := xraycore.New(coreConfig)
	if err != nil {
		a.lastError = err.Error()
		return fmt.Errorf("xray: create embedded core: %w", err)
	}
	if err := addStreamHandlers(instance, cfg.StreamHandlers); err != nil {
		_ = instance.Close()
		a.lastError = err.Error()
		return err
	}
	if err := instance.Start(); err != nil {
		_ = instance.Close()
		a.lastError = err.Error()
		return fmt.Errorf("xray: start embedded core: %w", err)
	}
	a.instance = instance
	a.startedAt = time.Now()
	a.exitedAt = time.Time{}
	a.lastError = ""
	a.endpoints = append([]EndpointRef(nil), cfg.Endpoints...)
	return nil
}

func (a *xrayCoreEmbeddedAdapter) DialTCP(ctx context.Context, outboundTag string, host string, port uint16) (net.Conn, error) {
	a.mu.Lock()
	instance := a.instance
	a.mu.Unlock()
	if instance == nil {
		return nil, fmt.Errorf("xray: embedded core is not running")
	}
	if host == "" {
		host = "tapx.frame.local"
	}
	if port == 0 {
		port = 1
	}
	if outboundTag != "" {
		ctx = session.SetForcedOutboundTagToContext(ctx, outboundTag)
	}
	return xraycore.Dial(ctx, instance, xnet.TCPDestination(xnet.ParseAddress(host), xnet.Port(port)))
}

func addStreamHandlers(instance *xraycore.Instance, handlers map[string]StreamHandler) error {
	if len(handlers) == 0 {
		return nil
	}
	manager, ok := instance.GetFeature(outbound.ManagerType()).(outbound.Manager)
	if !ok || manager == nil {
		return fmt.Errorf("xray: outbound manager is not available")
	}
	for tag, handler := range handlers {
		if err := manager.AddHandler(context.Background(), &streamOutboundHandler{tag: tag, handler: handler}); err != nil {
			return fmt.Errorf("xray: add stream handler %s: %w", tag, err)
		}
	}
	return nil
}

type streamOutboundHandler struct {
	tag     string
	handler StreamHandler
}

func (h *streamOutboundHandler) Start() error { return nil }

func (h *streamOutboundHandler) Close() error { return nil }

func (h *streamOutboundHandler) Tag() string { return h.tag }

func (h *streamOutboundHandler) SenderSettings() *serial.TypedMessage { return nil }

func (h *streamOutboundHandler) ProxySettings() *serial.TypedMessage { return nil }

func (h *streamOutboundHandler) Dispatch(ctx context.Context, link *transport.Link) {
	conn := cnc.NewConnection(
		cnc.ConnectionInputMulti(link.Writer),
		cnc.ConnectionOutputMulti(link.Reader),
	)
	defer conn.Close()
	h.handler(ctx, conn)
}

func (a *xrayCoreEmbeddedAdapter) Stop() error {
	a.mu.Lock()
	instance := a.instance
	if instance == nil {
		a.mu.Unlock()
		return nil
	}
	a.instance = nil
	a.endpoints = nil
	a.mu.Unlock()

	err := instance.Close()

	a.mu.Lock()
	defer a.mu.Unlock()
	a.exitedAt = time.Now()
	if err != nil {
		a.lastError = err.Error()
	}
	return err
}

func (a *xrayCoreEmbeddedAdapter) State() EmbeddedState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return EmbeddedState{
		Running:   a.instance != nil,
		Adapter:   "xray-core",
		StartedAt: a.startedAt,
		ExitedAt:  a.exitedAt,
		LastError: a.lastError,
		Endpoints: append([]EndpointRef(nil), a.endpoints...),
	}
}
