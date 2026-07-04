//go:build !linux

package core

import (
	"errors"
	"net/netip"

	"tapx/internal/config"
	"tapx/internal/fastpath"
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
}

type XrayPipeHandle struct {
	Pipe       config.RuntimeXrayPipe
	DeviceName string
}

func startUDPPipe(config.RuntimeUDPPipe, config.RuntimeDevice) (*UDPPipeHandle, error) {
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

func startTCPPipe(config.RuntimeTCPPipe, config.RuntimeDevice) (*TCPPipeHandle, error) {
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

func startXrayPipe(config.RuntimeXrayPipe, config.RuntimeDevice, *xrayruntime.Manager) (*XrayPipeHandle, error) {
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
