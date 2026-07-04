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
