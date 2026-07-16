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

func TestWriteFrameHandlesShortWrites(t *testing.T) {
	writer := &shortWriter{limit: 1}
	want := []byte{1, 2, 3, 4, 5}
	if err := WriteFrame(writer, model.TCPLength16, want); err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}
	got, err := ReadFrame(bytes.NewReader(writer.Bytes()), model.TCPLength16, len(want))
	if err != nil {
		t.Fatalf("ReadFrame() error = %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("round trip = %v, want %v", got, want)
	}
}

type shortWriter struct {
	bytes.Buffer
	limit int
}

func (w *shortWriter) Write(payload []byte) (int, error) {
	if len(payload) > w.limit {
		payload = payload[:w.limit]
	}
	return w.Buffer.Write(payload)
}

func TestReadFrameIntoReusesCallerBuffer(t *testing.T) {
	want := bytes.Repeat([]byte{0x5a}, 1500)
	var wire bytes.Buffer
	if err := WriteFrame(&wire, model.TCPLength16, want); err != nil {
		t.Fatal(err)
	}
	encoded := append([]byte(nil), wire.Bytes()...)
	dst := make([]byte, len(want))
	var reader bytes.Reader
	allocations := testing.AllocsPerRun(1000, func() {
		reader.Reset(encoded)
		got, err := ReadFrameInto(&reader, model.TCPLength16, dst, len(dst))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("ReadFrameInto() = %d bytes, %v", len(got), err)
		}
	})
	if allocations != 0 {
		t.Fatalf("ReadFrameInto() allocations = %v, want 0", allocations)
	}
}

func TestReadFrameIntoRejectsSmallDestination(t *testing.T) {
	var wire bytes.Buffer
	if err := WriteFrame(&wire, model.TCPLength16, bytes.Repeat([]byte{1}, 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFrameInto(&wire, model.TCPLength16, make([]byte, 32), 64); err == nil {
		t.Fatal("ReadFrameInto() error = nil, want destination buffer error")
	}
}

func TestDecodeFrameHeader(t *testing.T) {
	if got, err := DecodeFrameHeader([]byte{0x05, 0xdc}, model.TCPLength16, 1600); err != nil || got != 1500 {
		t.Fatalf("DecodeFrameHeader(uint16) = %d, %v", got, err)
	}
	if got, err := DecodeFrameHeader([]byte{0, 1, 0, 0}, model.TCPLength32, 65536); err != nil || got != 65536 {
		t.Fatalf("DecodeFrameHeader(uint32) = %d, %v", got, err)
	}
	if _, err := DecodeFrameHeader([]byte{0x05, 0xdc}, model.TCPLength16, 1400); err == nil {
		t.Fatal("DecodeFrameHeader() accepted an oversized frame")
	}
}

func BenchmarkReadFrameInto1500(b *testing.B) {
	payload := bytes.Repeat([]byte{0x7f}, 1500)
	var wire bytes.Buffer
	if err := WriteFrame(&wire, model.TCPLength16, payload); err != nil {
		b.Fatal(err)
	}
	encoded := append([]byte(nil), wire.Bytes()...)
	dst := make([]byte, len(payload))
	var reader bytes.Reader
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	for i := 0; i < b.N; i++ {
		reader.Reset(encoded)
		if _, err := ReadFrameInto(&reader, model.TCPLength16, dst, len(dst)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadFrameAllocated1500(b *testing.B) {
	payload := bytes.Repeat([]byte{0x7f}, 1500)
	var wire bytes.Buffer
	if err := WriteFrame(&wire, model.TCPLength16, payload); err != nil {
		b.Fatal(err)
	}
	encoded := append([]byte(nil), wire.Bytes()...)
	var reader bytes.Reader
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	for i := 0; i < b.N; i++ {
		reader.Reset(encoded)
		if _, err := ReadFrame(&reader, model.TCPLength16, len(payload)); err != nil {
			b.Fatal(err)
		}
	}
}
