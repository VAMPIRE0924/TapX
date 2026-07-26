//go:build linux

package core

import (
	"sync"

	"tapx/internal/netapply"
	"tapx/internal/tuntap"
)

type sharedRuntimeDevice struct {
	device   tuntap.Device
	netApply netapply.Handle
	address  *deviceAddressControl

	mu     sync.Mutex
	active bool
}

type tcpSharedDevice = sharedRuntimeDevice
type xraySharedDevice = sharedRuntimeDevice
