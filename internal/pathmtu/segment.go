package pathmtu

import (
	"encoding/binary"
	"fmt"
)

const (
	SegmentMaxFragments = 256
	ReassemblySlots     = 8
)

var segmentMagic = [4]byte{'T', 'X', 'S', '1'}

type Segmenter struct {
	maxPayload int
	sequence   uint32
	buffer     []byte
}

func NewSegmenter(maxPayload int) (*Segmenter, error) {
	if maxPayload <= SegmentHeaderSize || maxPayload > MaxUDPProbePayload {
		return nil, fmt.Errorf("segment payload limit must be between %d and %d", SegmentHeaderSize+1, MaxUDPProbePayload)
	}
	return &Segmenter{maxPayload: maxPayload, buffer: make([]byte, maxPayload)}, nil
}

// ShrinkMaxPayload lowers the datagram ceiling while preserving the sequence
// space used by an active reassembly peer. It is intended for runtime PMTU
// reductions and deliberately rejects increases, which require peer
// confirmation before use.
func (s *Segmenter) ShrinkMaxPayload(maxPayload int) error {
	if s == nil {
		return fmt.Errorf("segmenter is required")
	}
	if maxPayload <= SegmentHeaderSize {
		return fmt.Errorf("segment payload limit must be greater than %d", SegmentHeaderSize)
	}
	if maxPayload > s.maxPayload {
		return fmt.Errorf("segment payload limit cannot increase from %d to %d without peer confirmation", s.maxPayload, maxPayload)
	}
	s.maxPayload = maxPayload
	return nil
}

func (s *Segmenter) MaxPayload() int {
	if s == nil {
		return 0
	}
	return s.maxPayload
}

// WriteFrame emits one or more TXS1 datagrams. The callback must not retain the
// provided slice because the segmenter reuses one preallocated buffer.
func (s *Segmenter) WriteFrame(frame []byte, write func([]byte) error) error {
	if s == nil || write == nil {
		return fmt.Errorf("segmenter and write callback are required")
	}
	if len(frame) == 0 || len(frame) > 65535 {
		return fmt.Errorf("frame size must be between 1 and 65535")
	}
	fragmentPayload := s.maxPayload - SegmentHeaderSize
	fragmentCount := (len(frame) + fragmentPayload - 1) / fragmentPayload
	if fragmentCount > SegmentMaxFragments {
		return fmt.Errorf("frame requires %d fragments, limit is %d", fragmentCount, SegmentMaxFragments)
	}
	s.sequence++
	for index := 0; index < fragmentCount; index++ {
		offset := index * fragmentPayload
		fragmentLen := len(frame) - offset
		if fragmentLen > fragmentPayload {
			fragmentLen = fragmentPayload
		}
		packet := s.buffer[:SegmentHeaderSize+fragmentLen]
		writeSegmentHeader(packet, s.sequence, len(frame), index, fragmentCount, fragmentPayload, fragmentLen)
		copy(packet[SegmentHeaderSize:], frame[offset:offset+fragmentLen])
		if err := write(packet); err != nil {
			return err
		}
	}
	return nil
}

type reassemblySlot struct {
	sequence        uint32
	totalLen        int
	fragmentCount   int
	fragmentPayload int
	receivedCount   int
	active          bool
	received        [SegmentMaxFragments / 8]byte
	data            []byte
}

type Reassembler struct {
	maxFrame int
	slots    [ReassemblySlots]reassemblySlot
}

func NewReassembler(maxFrame int) (*Reassembler, error) {
	if maxFrame <= 0 || maxFrame > 65535 {
		return nil, fmt.Errorf("reassembly frame limit must be between 1 and 65535")
	}
	r := &Reassembler{maxFrame: maxFrame}
	for index := range r.slots {
		r.slots[index].data = make([]byte, maxFrame)
	}
	return r, nil
}

// Push returns an internal frame view only when all fragments have arrived.
// The caller must consume it before enough later sequences reuse the same slot.
func (r *Reassembler) Push(packet []byte) ([]byte, bool, error) {
	if r == nil {
		return nil, false, fmt.Errorf("reassembler is required")
	}
	sequence, totalLen, fragmentIndex, fragmentCount, fragmentPayload, fragmentLen, err := parseSegmentHeader(packet)
	if err != nil {
		return nil, false, err
	}
	if totalLen > r.maxFrame {
		return nil, false, fmt.Errorf("segment frame length %d exceeds %d", totalLen, r.maxFrame)
	}
	expectedCount := (totalLen + fragmentPayload - 1) / fragmentPayload
	offset := fragmentIndex * fragmentPayload
	if expectedCount != fragmentCount || offset >= totalLen {
		return nil, false, fmt.Errorf("segment layout is inconsistent")
	}
	expectedLen := totalLen - offset
	if expectedLen > fragmentPayload {
		expectedLen = fragmentPayload
	}
	if fragmentLen != expectedLen || len(packet) != SegmentHeaderSize+fragmentLen {
		return nil, false, fmt.Errorf("segment fragment length is inconsistent")
	}

	slot := &r.slots[sequence%ReassemblySlots]
	if !slot.active || slot.sequence != sequence || slot.totalLen != totalLen ||
		slot.fragmentCount != fragmentCount || slot.fragmentPayload != fragmentPayload {
		slot.sequence = sequence
		slot.totalLen = totalLen
		slot.fragmentCount = fragmentCount
		slot.fragmentPayload = fragmentPayload
		slot.receivedCount = 0
		slot.active = true
		clear(slot.received[:])
	}
	mask := byte(1 << (fragmentIndex & 7))
	bitmap := &slot.received[fragmentIndex>>3]
	if *bitmap&mask != 0 {
		return nil, false, nil
	}
	copy(slot.data[offset:offset+fragmentLen], packet[SegmentHeaderSize:])
	*bitmap |= mask
	slot.receivedCount++
	if slot.receivedCount != slot.fragmentCount {
		return nil, false, nil
	}
	slot.active = false
	return slot.data[:slot.totalLen], true, nil
}

func writeSegmentHeader(packet []byte, sequence uint32, totalLen, fragmentIndex, fragmentCount, fragmentPayload, fragmentLen int) {
	copy(packet[:4], segmentMagic[:])
	binary.BigEndian.PutUint32(packet[4:8], sequence)
	binary.BigEndian.PutUint16(packet[8:10], uint16(totalLen))
	binary.BigEndian.PutUint16(packet[10:12], uint16(fragmentIndex))
	binary.BigEndian.PutUint16(packet[12:14], uint16(fragmentCount))
	binary.BigEndian.PutUint16(packet[14:16], uint16(fragmentPayload))
	binary.BigEndian.PutUint16(packet[16:18], uint16(fragmentLen))
	clear(packet[18:20])
}

func parseSegmentHeader(packet []byte) (uint32, int, int, int, int, int, error) {
	if len(packet) < SegmentHeaderSize || string(packet[:4]) != string(segmentMagic[:]) || packet[18] != 0 || packet[19] != 0 {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("segment header is invalid")
	}
	sequence := binary.BigEndian.Uint32(packet[4:8])
	totalLen := int(binary.BigEndian.Uint16(packet[8:10]))
	fragmentIndex := int(binary.BigEndian.Uint16(packet[10:12]))
	fragmentCount := int(binary.BigEndian.Uint16(packet[12:14]))
	fragmentPayload := int(binary.BigEndian.Uint16(packet[14:16]))
	fragmentLen := int(binary.BigEndian.Uint16(packet[16:18]))
	if totalLen == 0 || fragmentCount == 0 || fragmentCount > SegmentMaxFragments ||
		fragmentIndex >= fragmentCount || fragmentPayload == 0 || fragmentLen == 0 {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("segment values are invalid")
	}
	return sequence, totalLen, fragmentIndex, fragmentCount, fragmentPayload, fragmentLen, nil
}
