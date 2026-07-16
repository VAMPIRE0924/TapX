//go:build linux

package core

import (
	"context"
	"errors"
	"testing"

	"tapx/internal/tuntap"
)

type readyDevice struct {
	payload []byte
	reads   int
}

var _ tuntap.Device = (*readyDevice)(nil)

func (d *readyDevice) Name() string { return "ready-test" }

func (d *readyDevice) FD() int {
	panic("readDeviceFrame called FD even though Read succeeded")
}

func (d *readyDevice) Read(dst []byte) (int, error) {
	d.reads++
	return copy(dst, d.payload), nil
}

func (d *readyDevice) Write([]byte) (int, error) { return 0, errors.New("unexpected write") }

func (d *readyDevice) Close() error { return nil }

func TestReadDeviceFrameReadsReadyQueueBeforePolling(t *testing.T) {
	device := &readyDevice{payload: []byte("ready")}
	buffer := make([]byte, 32)
	n, err := readDeviceFrame(context.Background(), device, buffer)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(buffer[:n]); got != "ready" {
		t.Fatalf("payload = %q, want ready", got)
	}
	if device.reads != 1 {
		t.Fatalf("Read calls = %d, want 1", device.reads)
	}
}
