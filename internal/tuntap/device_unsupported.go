//go:build !linux

package tuntap

import "errors"

func Open(OpenOptions) (Device, error) {
	return nil, errors.New("tuntap: only linux is supported")
}
