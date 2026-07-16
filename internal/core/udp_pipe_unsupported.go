//go:build !linux

package core

import (
	"context"
	"errors"
	"net/netip"
	"time"

	"tapx/internal/config"
	"tapx/internal/fastpath"
	"tapx/internal/pathmtu"
	"tapx/internal/xrayruntime"
)

type UDPPipeHandle struct {
	Pipe       config.RuntimeUDPPipe
	LocalAddr  netip.AddrPort
	RemoteAddr netip.AddrPort
	DeviceName string
	udpFD      int
}

type TCPPipeHandle struct {
	Pipe       config.RuntimeTCPPipe
	LocalAddr  netip.AddrPort
	RemoteAddr netip.AddrPort
	DeviceName string
	shared     *tcpSharedDevice
	owner      bool
}

type tcpSharedDevice struct{}

type XrayPipeHandle struct {
	Pipe       config.RuntimeXrayPipe
	DeviceName string
	shared     *xraySharedDevice
	owner      bool
}

type xraySharedDevice struct{}

type udpReuseportGroup struct {
	Dispatch config.RuntimeUDPDispatch
}

type DTLSDispatchHandle struct {
	Dispatch config.RuntimeUDPDispatch
}

func (g *udpReuseportGroup) Close() error { return nil }

func startUDPReuseportGroup(config.RuntimeUDPDispatch, config.RuntimeUDPPipe) (*udpReuseportGroup, error) {
	return nil, errors.New("core: UDP reuseport dispatch requires linux")
}

func startDTLSDispatch(config.RuntimeUDPDispatch, []config.RuntimeUDPPipe, []config.RuntimeDevice, *pathmtu.Cache) (*DTLSDispatchHandle, []*UDPPipeHandle, error) {
	return nil, nil, errors.New("core: DTLS dispatch requires linux")
}

func (h *DTLSDispatchHandle) Close() error { return nil }

func startUDPPipeWithCache(config.RuntimeUDPPipe, config.RuntimeDevice, *pathmtu.Cache) (*UDPPipeHandle, error) {
	return nil, errors.New("core: udp pipe supervisor requires linux")
}

func (h *UDPPipeHandle) Close() error {
	return nil
}

func (h *UDPPipeHandle) Counters() fastpath.CountersSnapshot {
	return fastpath.CountersSnapshot{}
}

func (h *UDPPipeHandle) Err() error {
	return nil
}

func (h *UDPPipeHandle) AcceptedRemoteAddr() netip.AddrPort {
	return netip.AddrPort{}
}

func (h *UDPPipeHandle) Diagnose(context.Context, string, time.Duration) (ConnectorDiagnostic, error) {
	return ConnectorDiagnostic{}, errors.New("core: UDP connector diagnostics require linux")
}

func startTCPPipe(config.RuntimeTCPPipe, config.RuntimeDevice) (*TCPPipeHandle, error) {
	return nil, errors.New("core: tcp pipe supervisor requires linux")
}

func startTCPPipeShared(config.RuntimeTCPPipe, config.RuntimeDevice, *tcpSharedDevice) (*TCPPipeHandle, error) {
	return nil, errors.New("core: tcp pipe supervisor requires linux")
}

func (h *TCPPipeHandle) Close() error {
	return nil
}

func (h *TCPPipeHandle) Counters() fastpath.CountersSnapshot {
	return fastpath.CountersSnapshot{}
}

func (h *TCPPipeHandle) Err() error {
	return nil
}

func (h *TCPPipeHandle) AcceptedRemoteAddr() netip.AddrPort {
	return netip.AddrPort{}
}

func (h *TCPPipeHandle) Diagnose(context.Context, string, time.Duration) (ConnectorDiagnostic, error) {
	return ConnectorDiagnostic{}, errors.New("core: connector diagnostics require linux")
}

func startXrayPipeShared(config.RuntimeXrayPipe, config.RuntimeDevice, *xrayruntime.Manager, *xraySharedDevice) (*XrayPipeHandle, error) {
	return nil, errors.New("core: xray pipe supervisor requires linux")
}

func (h *XrayPipeHandle) Close() error {
	return nil
}

func (h *XrayPipeHandle) Counters() fastpath.CountersSnapshot {
	return fastpath.CountersSnapshot{}
}

func (h *XrayPipeHandle) Err() error {
	return nil
}

func (h *XrayPipeHandle) Diagnose(context.Context, string, time.Duration) (ConnectorDiagnostic, error) {
	return ConnectorDiagnostic{}, errors.New("core: xray connector diagnostics require linux")
}
