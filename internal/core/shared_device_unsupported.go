//go:build !linux

package core

type sharedRuntimeDevice struct{}
type tcpSharedDevice = sharedRuntimeDevice
type xraySharedDevice = sharedRuntimeDevice
