//go:build linux

package core

import (
	"bytes"
	"io"
	"testing"

	xbuf "github.com/xtls/xray-core/common/buf"

	"tapx/internal/model"
	"tapx/internal/rawtcp"
)

type sequenceMultiBufferReader struct {
	batches []xbuf.MultiBuffer
	index   int
}

func (r *sequenceMultiBufferReader) ReadMultiBuffer() (xbuf.MultiBuffer, error) {
	if r.index >= len(r.batches) {
		return nil, io.EOF
	}
	batch := r.batches[r.index]
	r.index++
	if r.index == len(r.batches) {
		return batch, io.EOF
	}
	return batch, nil
}

func TestXrayMultiBufferFrameReaderPreservesCoalescedBoundaries(t *testing.T) {
	first := bytes.Repeat([]byte{0x11}, 96)
	second := bytes.Repeat([]byte{0x22}, 128)
	wire := appendFrame(nil, model.TCPLength16, first)
	wire = appendFrame(wire, model.TCPLength16, second)
	reader := &sequenceMultiBufferReader{batches: []xbuf.MultiBuffer{{xrayTestBuffer(wire)}}}
	framed := xrayMultiBufferFrameReader{reader: reader}
	defer framed.Close()
	scratch := make([]byte, 256)

	gotFirst, release, err := framed.ReadFrame(model.TCPLength16, scratch, len(scratch))
	if err != nil {
		t.Fatal(err)
	}
	if release != nil {
		t.Fatal("first coalesced frame unexpectedly consumed its pooled buffer")
	}
	if !bytes.Equal(gotFirst, first) {
		t.Fatal("first coalesced frame mismatch")
	}

	gotSecond, release, err := framed.ReadFrame(model.TCPLength16, scratch, len(scratch))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotSecond, second) {
		t.Fatal("second coalesced frame mismatch")
	}
	if release == nil {
		t.Fatal("final coalesced frame did not return its consumed pooled buffer")
	}
	release.Release()
	if _, _, err := framed.ReadFrame(model.TCPLength16, scratch, len(scratch)); err != io.EOF {
		t.Fatalf("terminal error = %v, want EOF", err)
	}
}

func TestXrayMultiBufferFrameReaderReassemblesFragmentedFrame(t *testing.T) {
	payload := bytes.Repeat([]byte{0xa5}, 1500)
	wire := appendFrame(nil, model.TCPLength32, payload)
	reader := &sequenceMultiBufferReader{batches: []xbuf.MultiBuffer{
		{xrayTestBuffer(wire[:1])},
		{xrayTestBuffer(wire[1:503]), xrayTestBuffer(wire[503:])},
	}}
	framed := xrayMultiBufferFrameReader{reader: reader}
	defer framed.Close()
	scratch := make([]byte, len(payload))

	got, release, err := framed.ReadFrame(model.TCPLength32, scratch, len(scratch))
	if err != nil {
		t.Fatal(err)
	}
	if release != nil {
		t.Fatal("fragmented frame unexpectedly returned a direct pooled buffer")
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("fragmented frame mismatch")
	}
}

func BenchmarkXrayMultiBufferFrameReaderContiguous1500(b *testing.B) {
	payload := bytes.Repeat([]byte{0x5a}, 1500)
	wire := appendFrame(nil, model.TCPLength16, payload)
	scratch := make([]byte, len(payload))
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	for b.Loop() {
		reader := &sequenceMultiBufferReader{batches: []xbuf.MultiBuffer{{xrayTestBuffer(wire)}}}
		framed := xrayMultiBufferFrameReader{reader: reader}
		frame, release, err := framed.ReadFrame(model.TCPLength16, scratch, len(payload))
		if err != nil || len(frame) != len(payload) {
			b.Fatalf("ReadFrame() = %d, %v", len(frame), err)
		}
		if release != nil {
			release.Release()
		}
		framed.Close()
	}
}

func BenchmarkXrayMultiBufferFrameReaderFragmented1500(b *testing.B) {
	payload := bytes.Repeat([]byte{0xa5}, 1500)
	wire := appendFrame(nil, model.TCPLength16, payload)
	scratch := make([]byte, len(payload))
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	for b.Loop() {
		reader := &sequenceMultiBufferReader{batches: []xbuf.MultiBuffer{
			{xrayTestBuffer(wire[:501])},
			{xrayTestBuffer(wire[501:1001])},
			{xrayTestBuffer(wire[1001:])},
		}}
		framed := xrayMultiBufferFrameReader{reader: reader}
		frame, release, err := framed.ReadFrame(model.TCPLength16, scratch, len(payload))
		if err != nil || len(frame) != len(payload) {
			b.Fatalf("ReadFrame() = %d, %v", len(frame), err)
		}
		if release != nil {
			release.Release()
		}
		framed.Close()
	}
}

func appendFrame(dst []byte, mode model.TCPLengthMode, payload []byte) []byte {
	var header [4]byte
	n, err := rawtcp.EncodeFrameHeader(header[:], mode, len(payload))
	if err != nil {
		panic(err)
	}
	dst = append(dst, header[:n]...)
	return append(dst, payload...)
}

func xrayTestBuffer(payload []byte) *xbuf.Buffer {
	buffer := xbuf.NewWithSize(int32(len(payload)))
	copy(buffer.Extend(int32(len(payload))), payload)
	return buffer
}
