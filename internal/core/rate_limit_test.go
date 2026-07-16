package core

import (
	"io"
	"net"
	"testing"
	"time"

	"tapx/internal/config"
)

func TestUserTrafficRates(t *testing.T) {
	binding := config.RuntimeBinding{UploadRateLimit: 1_000_000, DownloadRateLimit: 2_000_000}
	readRate, writeRate := userTrafficRates("listener", binding)
	if readRate != 1_000_000 || writeRate != 2_000_000 {
		t.Fatalf("listener rates = (%d, %d)", readRate, writeRate)
	}
	readRate, writeRate = userTrafficRates("connector", binding)
	if readRate != 2_000_000 || writeRate != 1_000_000 {
		t.Fatalf("connector rates = (%d, %d)", readRate, writeRate)
	}
}

func TestAddressGuardDirections(t *testing.T) {
	tests := []struct {
		remoteIdentity  bool
		deviceToNetwork bool
		wantSource      bool
	}{
		{remoteIdentity: true, deviceToNetwork: false, wantSource: true},
		{remoteIdentity: true, deviceToNetwork: true, wantSource: false},
		{remoteIdentity: false, deviceToNetwork: false, wantSource: false},
		{remoteIdentity: false, deviceToNetwork: true, wantSource: true},
	}
	for _, tt := range tests {
		if got := addressGuardSource(tt.remoteIdentity, tt.deviceToNetwork); got != tt.wantSource {
			t.Fatalf("addressGuardSource(%v, %v) = %v, want %v", tt.remoteIdentity, tt.deviceToNetwork, got, tt.wantSource)
		}
	}
}

func TestApplyUserRateLimitsLeavesUnlimitedConnectionUntouched(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	if got := applyUserRateLimits(left, "listener", config.RuntimeBinding{}); got != left {
		t.Fatal("unlimited connection was wrapped")
	}
}

func TestRateLimitedConnPacesWrites(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	limited := newRateLimitedConn(left, 0, 80_000)
	payload := make([]byte, 2_000)
	done := make(chan error, 1)
	go func() {
		_, err := io.CopyN(io.Discard, right, int64(len(payload)*2))
		done <- err
	}()
	started := time.Now()
	if _, err := limited.Write(payload); err != nil {
		t.Fatal(err)
	}
	if _, err := limited.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed < 150*time.Millisecond || elapsed > 2*time.Second {
		t.Fatalf("paced writes elapsed %s", elapsed)
	}
}
