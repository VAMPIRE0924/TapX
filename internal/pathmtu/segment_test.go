package pathmtu

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestSegmenterReassemblesOutOfOrderAndIgnoresDuplicate(t *testing.T) {
	segmenter, err := NewSegmenter(128)
	if err != nil {
		t.Fatal(err)
	}
	frame := make([]byte, 300)
	for index := range frame {
		frame[index] = byte(index)
	}
	var packets [][]byte
	if err := segmenter.WriteFrame(frame, func(packet []byte) error {
		packets = append(packets, append([]byte(nil), packet...))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(packets) != 3 || len(packets[0]) != 128 || len(packets[2]) != 104 {
		t.Fatalf("packet lengths = %d/%d/%d", len(packets), len(packets[0]), len(packets[2]))
	}

	reassembler, err := NewReassembler(1500)
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range []int{2, 0, 0} {
		if _, complete, err := reassembler.Push(packets[index]); err != nil || complete {
			t.Fatalf("Push(%d) = complete %v, err %v", index, complete, err)
		}
	}
	got, complete, err := reassembler.Push(packets[1])
	if err != nil || !complete {
		t.Fatalf("final Push() = complete %v, err %v", complete, err)
	}
	if !bytes.Equal(got, frame) {
		t.Fatal("reassembled frame does not match")
	}
}

func TestReassemblerRejectsMalformedSegment(t *testing.T) {
	segmenter, err := NewSegmenter(128)
	if err != nil {
		t.Fatal(err)
	}
	var packet []byte
	if err := segmenter.WriteFrame(make([]byte, 64), func(value []byte) error {
		packet = append([]byte(nil), value...)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	packet[16] = 0
	packet[17] = 1
	reassembler, err := NewReassembler(1500)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := reassembler.Push(packet); err == nil {
		t.Fatal("malformed segment was accepted")
	}
}

func TestSegmenterShrinksWithoutResettingSequence(t *testing.T) {
	segmenter, err := NewSegmenter(128)
	if err != nil {
		t.Fatal(err)
	}
	var first [][]byte
	if err := segmenter.WriteFrame(make([]byte, 180), func(packet []byte) error {
		first = append(first, append([]byte(nil), packet...))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := segmenter.ShrinkMaxPayload(96); err != nil {
		t.Fatal(err)
	}
	if got := segmenter.MaxPayload(); got != 96 {
		t.Fatalf("MaxPayload() = %d, want 96", got)
	}
	var second [][]byte
	if err := segmenter.WriteFrame(make([]byte, 180), func(packet []byte) error {
		second = append(second, append([]byte(nil), packet...))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(first) == 0 || len(second) == 0 || binary.BigEndian.Uint32(first[0][4:8]) != 1 ||
		binary.BigEndian.Uint32(second[0][4:8]) != 2 {
		t.Fatalf("segment sequences did not continue across shrink")
	}
	for _, packet := range second {
		if len(packet) > 96 {
			t.Fatalf("shrunk segment length = %d, want <= 96", len(packet))
		}
	}
	if err := segmenter.ShrinkMaxPayload(97); err == nil {
		t.Fatal("segmenter accepted an unconfirmed increase")
	}
}
