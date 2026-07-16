//go:build !linux

package main

import (
	"fmt"
	"net"
	"strings"
)

func listenPanel(address, interfaceName string) (net.Listener, error) {
	if strings.TrimSpace(interfaceName) != "" {
		return nil, fmt.Errorf("binding the panel to an interface is supported only on Linux")
	}
	return net.Listen("tcp", address)
}
