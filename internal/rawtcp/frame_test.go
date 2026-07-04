package rawtcp

import (
	"bytes"
	"testing"

	"tapx/internal/model"
)

func TestFrameRoundTripUint16(t *testing.T) {
	var buf bytes.Buffer
	want := []byte{0x45, 0x00, 0x00, 0x14}
	if err := WriteFrame(&buf, model.TCPLength16, want); err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}
	got, err := ReadFrame(&buf, model.TCPLength16, 1500)
	if err != nil {
		t.Fatalf("ReadFrame() error = %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("ReadFrame() = %v, want %v", got, want)
	}
}

func TestFrameRoundTripUint32(t *testing.T) {
	var buf bytes.Buffer
	want := bytes.Repeat([]byte{0xab}, 70000)
	if err := WriteFrame(&buf, model.TCPLength32, want); err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}
	got, err := ReadFrame(&buf, model.TCPLength32, 80000)
	if err != nil {
		t.Fatalf("ReadFrame() error = %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("ReadFrame() length = %d, want %d", len(got), len(want))
	}
}

func TestFrameRejectsOversize(t *testing.T) {
	var buf bytes.Buffer
	err := WriteFrame(&buf, model.TCPLength16, bytes.Repeat([]byte{0}, 70000))
	if err == nil {
		t.Fatal("WriteFrame() error = nil, want oversize error")
	}
}
