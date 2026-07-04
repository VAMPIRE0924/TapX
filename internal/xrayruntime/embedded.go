package xrayruntime

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

type EmbeddedConfig struct {
	Endpoints      []EndpointRef
	Document       map[string]any
	StreamHandlers map[string]StreamHandler
}

type EmbeddedState struct {
	Running   bool
	Adapter   string
	StartedAt time.Time
	ExitedAt  time.Time
	LastError string
	Endpoints []EndpointRef
}

type EmbeddedAdapter interface {
	Start(EmbeddedConfig) error
	Stop() error
	State() EmbeddedState
}

type StreamHandler func(context.Context, net.Conn)

type EmbeddedDialer interface {
	DialTCP(context.Context, string, string, uint16) (net.Conn, error)
}

type prototypeEmbeddedAdapter struct {
	mu        sync.Mutex
	running   bool
	startedAt time.Time
	exitedAt  time.Time
	lastError string
	endpoints []EndpointRef
}

func NewPrototypeEmbeddedAdapter() EmbeddedAdapter {
	return &prototypeEmbeddedAdapter{}
}

func (a *prototypeEmbeddedAdapter) Start(cfg EmbeddedConfig) error {
	if len(cfg.Endpoints) == 0 {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running {
		return fmt.Errorf("xray: embedded adapter already started")
	}
	a.running = true
	a.startedAt = time.Now()
	a.exitedAt = time.Time{}
	a.lastError = ""
	a.endpoints = append([]EndpointRef(nil), cfg.Endpoints...)
	return nil
}

func (a *prototypeEmbeddedAdapter) Stop() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.running {
		return nil
	}
	a.running = false
	a.exitedAt = time.Now()
	a.endpoints = nil
	return nil
}

func (a *prototypeEmbeddedAdapter) State() EmbeddedState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return EmbeddedState{
		Running:   a.running,
		Adapter:   "prototype",
		StartedAt: a.startedAt,
		ExitedAt:  a.exitedAt,
		LastError: a.lastError,
		Endpoints: append([]EndpointRef(nil), a.endpoints...),
	}
}
