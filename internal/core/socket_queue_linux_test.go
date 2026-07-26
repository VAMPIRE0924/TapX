//go:build linux

package core

import "testing"

func TestSocketQueueBytes(t *testing.T) {
	if got := socketQueueBytes(0, 1500); got != 0 {
		t.Fatalf("disabled queue = %d, want 0", got)
	}
	if got := socketQueueBytes(2048, 1500); got != 2048*(1500+64) {
		t.Fatalf("queue = %d", got)
	}
	if got := socketQueueBytes(1<<30, 65535); got != maxAutomaticSocketQueueBytes {
		t.Fatalf("bounded queue = %d, want %d", got, maxAutomaticSocketQueueBytes)
	}
}
