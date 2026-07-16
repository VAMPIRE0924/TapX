//go:build !linux

package core

import (
	"errors"

	"tapx/internal/config"
)

type TCPDispatchHandle struct {
	Dispatch config.RuntimeTCPDispatch
}

func startTCPDispatch(config.RuntimeTCPDispatch, []config.RuntimeTCPPipe, []config.RuntimeDevice) (*TCPDispatchHandle, []*TCPPipeHandle, error) {
	return nil, nil, errors.New("core: TCP vKey dispatch requires linux")
}

func (h *TCPDispatchHandle) Close() error { return nil }
