//go:build linux

package tuntap

import (
	"os"
	"testing"

	"tapx/internal/model"
)

func TestOpenTUNOptional(t *testing.T) {
	if os.Getenv("TAPX_TEST_TUNTAP") != "1" {
		t.Skip("set TAPX_TEST_TUNTAP=1 to create a real TUN device")
	}
	dev, err := Open(OpenOptions{
		Name:     "tapxtun%d",
		Type:     model.DeviceTUN,
		NonBlock: true,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer dev.Close()
	if dev.Name() == "" {
		t.Fatal("Open() returned empty device name")
	}
	if dev.FD() < 0 {
		t.Fatalf("Open() fd = %d, want >= 0", dev.FD())
	}
}
