//go:build linux

package main

import (
	"context"
	"fmt"
	"net"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

func listenPanel(address, interfaceName string) (net.Listener, error) {
	interfaceName = strings.TrimSpace(interfaceName)
	if interfaceName == "" {
		return net.Listen("tcp", address)
	}
	if _, err := net.InterfaceByName(interfaceName); err != nil {
		return nil, fmt.Errorf("listen interface %q: %w", interfaceName, err)
	}
	listenConfig := net.ListenConfig{
		Control: func(_, _ string, raw syscall.RawConn) error {
			var controlErr error
			if err := raw.Control(func(fd uintptr) {
				controlErr = unix.SetsockoptString(int(fd), unix.SOL_SOCKET, unix.SO_BINDTODEVICE, interfaceName)
			}); err != nil {
				return err
			}
			return controlErr
		},
	}
	listener, err := listenConfig.Listen(context.Background(), "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("listen on %s via %s: %w", address, interfaceName, err)
	}
	return listener, nil
}
