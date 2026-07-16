package pathmtu

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
)

const (
	ProbeHeaderSize = 24
	ProbeVersion    = 1
)

var probeMagic = [4]byte{'T', 'P', 'M', 'T'}

type ProbeType uint8

const (
	ProbeRequest   ProbeType = 1
	ProbeResponse  ProbeType = 2
	ProbeCommit    ProbeType = 3
	ProbeCommitted ProbeType = 4
)

type ProbeMessage struct {
	Type  ProbeType
	Token uint64
	Size  int
	Body  []byte
}

type ProbeExchange func(ctx context.Context, request []byte) ([]byte, error)

type ConfirmOptions struct {
	DesiredPayload   int
	CandidatePayload int
	MinimumPayload   int
	Attempts         int
}

type ConfirmResult struct {
	PayloadSize int
	ProbeCount  int
}

// NewProbeRequest builds one exact-size probe with a random identity and
// incompressible body. It is control-plane traffic and is never used when
// automatic link optimization is disabled.
func NewProbeRequest(size int) ([]byte, error) {
	if size < ProbeHeaderSize {
		return nil, fmt.Errorf("probe payload size %d is smaller than header %d", size, ProbeHeaderSize)
	}
	if uint64(size) > uint64(^uint32(0)) {
		return nil, fmt.Errorf("probe payload size %d exceeds protocol limit", size)
	}
	var tokenBytes [8]byte
	if _, err := rand.Read(tokenBytes[:]); err != nil {
		return nil, fmt.Errorf("generate probe token: %w", err)
	}
	packet := make([]byte, size)
	if _, err := rand.Read(packet[ProbeHeaderSize:]); err != nil {
		return nil, fmt.Errorf("generate probe body: %w", err)
	}
	writeProbeHeader(packet, ProbeRequest, binary.BigEndian.Uint64(tokenBytes[:]))
	return packet, nil
}

// ProbeResponseFor validates a request and returns an exact-size response.
// The random body is echoed unchanged so the initiator can reject stale or
// unrelated packets even when their token and length happen to match.
func ProbeResponseFor(request []byte) ([]byte, error) {
	return transformProbe(request, ProbeRequest, ProbeResponse)
}

func ProbeCommitFor(request []byte) ([]byte, error) {
	return transformProbe(request, ProbeRequest, ProbeCommit)
}

func ProbeCommittedFor(commit []byte) ([]byte, error) {
	return transformProbe(commit, ProbeCommit, ProbeCommitted)
}

func ParseProbe(packet []byte) (ProbeMessage, error) {
	if len(packet) < ProbeHeaderSize {
		return ProbeMessage{}, fmt.Errorf("probe packet is shorter than %d bytes", ProbeHeaderSize)
	}
	if !bytes.Equal(packet[:4], probeMagic[:]) {
		return ProbeMessage{}, errors.New("probe magic mismatch")
	}
	if packet[4] != ProbeVersion {
		return ProbeMessage{}, fmt.Errorf("unsupported probe version %d", packet[4])
	}
	probeType := ProbeType(packet[5])
	if probeType != ProbeRequest && probeType != ProbeResponse &&
		probeType != ProbeCommit && probeType != ProbeCommitted {
		return ProbeMessage{}, fmt.Errorf("unsupported probe type %d", probeType)
	}
	if headerSize := int(binary.BigEndian.Uint16(packet[6:8])); headerSize != ProbeHeaderSize {
		return ProbeMessage{}, fmt.Errorf("unsupported probe header size %d", headerSize)
	}
	declaredSize := int(binary.BigEndian.Uint32(packet[16:20]))
	if declaredSize != len(packet) {
		return ProbeMessage{}, fmt.Errorf("probe size %d does not match datagram length %d", declaredSize, len(packet))
	}
	wantChecksum := binary.BigEndian.Uint32(packet[20:24])
	copyForChecksum := append([]byte(nil), packet...)
	clear(copyForChecksum[20:24])
	if got := crc32.ChecksumIEEE(copyForChecksum); got != wantChecksum {
		return ProbeMessage{}, fmt.Errorf("probe checksum mismatch: got %08x want %08x", got, wantChecksum)
	}
	return ProbeMessage{
		Type:  probeType,
		Token: binary.BigEndian.Uint64(packet[8:16]),
		Size:  declaredSize,
		Body:  packet[ProbeHeaderSize:],
	}, nil
}

func transformProbe(packet []byte, from, to ProbeType) ([]byte, error) {
	message, err := ParseProbe(packet)
	if err != nil {
		return nil, err
	}
	if message.Type != from {
		return nil, fmt.Errorf("probe packet type %d is not %d", message.Type, from)
	}
	transformed := append([]byte(nil), packet...)
	writeProbeHeader(transformed, to, message.Token)
	return transformed, nil
}

// ConfirmPayload tries the operator ceiling first. When that fails it tries
// the kernel route candidate, establishes a known-good lower bound, and then
// performs a short binary search. A size is accepted only after receiving the
// exact response derived from that request.
func ConfirmPayload(ctx context.Context, options ConfirmOptions, exchange ProbeExchange) (ConfirmResult, error) {
	if exchange == nil {
		return ConfirmResult{}, errors.New("probe exchange is required")
	}
	if options.DesiredPayload < ProbeHeaderSize {
		return ConfirmResult{}, fmt.Errorf("desired payload must be at least %d", ProbeHeaderSize)
	}
	if options.MinimumPayload < ProbeHeaderSize || options.MinimumPayload > options.DesiredPayload {
		return ConfirmResult{}, fmt.Errorf("minimum payload must be between %d and desired payload", ProbeHeaderSize)
	}
	attempts := options.Attempts
	if attempts <= 0 {
		attempts = 3
	}
	candidate := options.CandidatePayload
	if candidate < options.MinimumPayload {
		candidate = options.MinimumPayload
	}
	if candidate > options.DesiredPayload {
		candidate = options.DesiredPayload
	}

	probeCount := 0
	var lastExchangeErr error
	try := func(size int) bool {
		for range attempts {
			if err := ctx.Err(); err != nil {
				return false
			}
			request, err := NewProbeRequest(size)
			if err != nil {
				return false
			}
			probeCount++
			response, err := exchange(ctx, request)
			if err != nil {
				lastExchangeErr = err
				continue
			}
			expected, err := ProbeResponseFor(request)
			if err == nil && bytes.Equal(response, expected) {
				return true
			}
		}
		return false
	}

	if try(options.DesiredPayload) {
		return ConfirmResult{PayloadSize: options.DesiredPayload, ProbeCount: probeCount}, nil
	}
	failedUpper := options.DesiredPayload
	knownGood := 0
	if candidate < failedUpper {
		if try(candidate) {
			knownGood = candidate
		} else {
			failedUpper = candidate
		}
	}
	if knownGood == 0 {
		if options.MinimumPayload == failedUpper || !try(options.MinimumPayload) {
			if err := ctx.Err(); err != nil {
				return ConfirmResult{ProbeCount: probeCount}, err
			}
			if lastExchangeErr != nil {
				return ConfirmResult{ProbeCount: probeCount}, fmt.Errorf("no peer-confirmed payload at or above %d bytes: %w", options.MinimumPayload, lastExchangeErr)
			}
			return ConfirmResult{ProbeCount: probeCount}, fmt.Errorf("no peer-confirmed payload at or above %d bytes", options.MinimumPayload)
		}
		knownGood = options.MinimumPayload
	}

	low, high := knownGood, failedUpper-1
	for low < high {
		mid := low + (high-low+1)/2
		if try(mid) {
			low = mid
		} else {
			high = mid - 1
		}
	}
	return ConfirmResult{PayloadSize: low, ProbeCount: probeCount}, nil
}

func writeProbeHeader(packet []byte, probeType ProbeType, token uint64) {
	copy(packet[:4], probeMagic[:])
	packet[4] = ProbeVersion
	packet[5] = byte(probeType)
	binary.BigEndian.PutUint16(packet[6:8], ProbeHeaderSize)
	binary.BigEndian.PutUint64(packet[8:16], token)
	binary.BigEndian.PutUint32(packet[16:20], uint32(len(packet)))
	clear(packet[20:24])
	binary.BigEndian.PutUint32(packet[20:24], crc32.ChecksumIEEE(packet))
}
