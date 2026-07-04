//go:build linux

package tuntap

import (
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"

	"tapx/internal/model"
)

const (
	tunSetIFF     = 0x400454ca
	iffTun        = 0x0001
	iffTap        = 0x0002
	iffNoPI       = 0x1000
	iffMultiQueue = 0x0100
)

type ifReq struct {
	Name  [unix.IFNAMSIZ]byte
	Flags uint16
	Pad   [22]byte
}

type linuxDevice struct {
	name string
	fd   int
}

func Open(opts OpenOptions) (Device, error) {
	if opts.Name == "" {
		return nil, errors.New("tuntap: device name is required")
	}
	flags := uint16(iffNoPI)
	switch opts.Type {
	case model.DeviceTUN:
		flags |= iffTun
	case model.DeviceTAP:
		flags |= iffTap
	default:
		return nil, fmt.Errorf("tuntap: unsupported device type %q", opts.Type)
	}
	if opts.MultiQueue {
		flags |= iffMultiQueue
	}

	openFlags := unix.O_RDWR | unix.O_CLOEXEC
	if opts.NonBlock {
		openFlags |= unix.O_NONBLOCK
	}
	fd, err := unix.Open("/dev/net/tun", openFlags, 0)
	if err != nil {
		return nil, err
	}

	var req ifReq
	copy(req.Name[:], opts.Name)
	req.Flags = flags
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(tunSetIFF), uintptr(unsafe.Pointer(&req)))
	if errno != 0 {
		_ = unix.Close(fd)
		return nil, errno
	}

	return &linuxDevice{name: ifName(req.Name[:]), fd: fd}, nil
}

func (d *linuxDevice) Name() string {
	return d.name
}

func (d *linuxDevice) FD() int {
	return d.fd
}

func (d *linuxDevice) Read(buf []byte) (int, error) {
	return unix.Read(d.fd, buf)
}

func (d *linuxDevice) Write(buf []byte) (int, error) {
	return unix.Write(d.fd, buf)
}

func (d *linuxDevice) Close() error {
	if d.fd < 0 {
		return nil
	}
	err := unix.Close(d.fd)
	d.fd = -1
	return err
}

func ifName(buf []byte) string {
	n := 0
	for n < len(buf) && buf[n] != 0 {
		n++
	}
	return strings.TrimSpace(string(buf[:n]))
}
