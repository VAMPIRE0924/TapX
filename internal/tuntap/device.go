package tuntap

import "tapx/internal/model"

type OpenOptions struct {
	Name       string
	Type       model.DeviceType
	MultiQueue bool
	NonBlock   bool
}

type Device interface {
	Name() string
	FD() int
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Close() error
}

// WrapFD exposes an already configured packet-oriented file descriptor as a
// Device. Ownership of fd is transferred to the returned Device.
func WrapFD(name string, fd int) Device {
	return wrapFD(name, fd)
}
