package rawtcp

import (
	"encoding/binary"
	"fmt"
	"io"

	"tapx/internal/model"
)

const DefaultMaxFrameSize = 65535

func WriteFrame(w io.Writer, mode model.TCPLengthMode, payload []byte) error {
	var header [4]byte
	headerSize, err := EncodeFrameHeader(header[:], mode, len(payload))
	if err != nil {
		return err
	}
	if err := writeFull(w, header[:headerSize]); err != nil {
		return err
	}
	return writeFull(w, payload)
}

func writeFull(w io.Writer, payload []byte) error {
	for len(payload) > 0 {
		n, err := w.Write(payload)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrNoProgress
		}
		payload = payload[n:]
	}
	return nil
}

func ReadFrame(r io.Reader, mode model.TCPLengthMode, maxSize int) ([]byte, error) {
	size, err := readFrameSize(r, mode, maxSize)
	if err != nil {
		return nil, err
	}
	payload := make([]byte, size)
	_, err = io.ReadFull(r, payload)
	return payload, err
}

// ReadFrameInto reads a frame into caller-owned storage. The returned slice is
// valid until the caller reuses dst.
func ReadFrameInto(r io.Reader, mode model.TCPLengthMode, dst []byte, maxSize int) ([]byte, error) {
	headerSize, err := FrameHeaderSize(mode)
	if err != nil {
		return nil, err
	}
	if len(dst) < headerSize {
		return nil, fmt.Errorf("rawtcp: destination buffer %d cannot hold frame header %d", len(dst), headerSize)
	}
	if _, err := io.ReadFull(r, dst[:headerSize]); err != nil {
		return nil, err
	}
	var size uint32
	if normalize(mode) == model.TCPLength16 {
		size = uint32(binary.BigEndian.Uint16(dst[:headerSize]))
	} else {
		size = binary.BigEndian.Uint32(dst[:headerSize])
	}
	if maxSize <= 0 {
		maxSize = DefaultMaxFrameSize
	}
	if size > uint32(maxSize) {
		return nil, fmt.Errorf("rawtcp: frame length %d exceeds max %d", size, maxSize)
	}
	if int(size) > len(dst) {
		return nil, fmt.Errorf("rawtcp: frame length %d exceeds destination buffer %d", size, len(dst))
	}
	payload := dst[:size]
	_, err = io.ReadFull(r, payload)
	return payload, err
}

func readFrameSize(r io.Reader, mode model.TCPLengthMode, maxSize int) (uint32, error) {
	if maxSize <= 0 {
		maxSize = DefaultMaxFrameSize
	}

	var size uint32
	switch normalize(mode) {
	case model.TCPLength16:
		var header [2]byte
		if _, err := io.ReadFull(r, header[:]); err != nil {
			return 0, err
		}
		size = uint32(binary.BigEndian.Uint16(header[:]))
	case model.TCPLength32:
		var header [4]byte
		if _, err := io.ReadFull(r, header[:]); err != nil {
			return 0, err
		}
		size = binary.BigEndian.Uint32(header[:])
	default:
		return 0, fmt.Errorf("rawtcp: unsupported length mode %q", mode)
	}

	if size > uint32(maxSize) {
		return 0, fmt.Errorf("rawtcp: frame length %d exceeds max %d", size, maxSize)
	}
	return size, nil
}

func FrameHeaderSize(mode model.TCPLengthMode) (int, error) {
	switch normalize(mode) {
	case model.TCPLength16:
		return 2, nil
	case model.TCPLength32:
		return 4, nil
	default:
		return 0, fmt.Errorf("rawtcp: unsupported length mode %q", mode)
	}
}

func EncodeFrameHeader(dst []byte, mode model.TCPLengthMode, payloadSize int) (int, error) {
	headerSize, err := FrameHeaderSize(mode)
	if err != nil {
		return 0, err
	}
	if len(dst) < headerSize {
		return 0, fmt.Errorf("rawtcp: frame header buffer %d is smaller than %d", len(dst), headerSize)
	}
	if payloadSize < 0 {
		return 0, fmt.Errorf("rawtcp: frame length %d is invalid", payloadSize)
	}
	switch normalize(mode) {
	case model.TCPLength16:
		if payloadSize > 65535 {
			return 0, fmt.Errorf("rawtcp: frame length %d exceeds uint16", payloadSize)
		}
		binary.BigEndian.PutUint16(dst[:headerSize], uint16(payloadSize))
	case model.TCPLength32:
		if uint64(payloadSize) > uint64(^uint32(0)) {
			return 0, fmt.Errorf("rawtcp: frame length %d exceeds uint32", payloadSize)
		}
		binary.BigEndian.PutUint32(dst[:headerSize], uint32(payloadSize))
	}
	return headerSize, nil
}

func DecodeFrameHeader(src []byte, mode model.TCPLengthMode, maxSize int) (int, error) {
	headerSize, err := FrameHeaderSize(mode)
	if err != nil {
		return 0, err
	}
	if len(src) < headerSize {
		return 0, fmt.Errorf("rawtcp: frame header buffer %d is smaller than %d", len(src), headerSize)
	}
	var size uint32
	if normalize(mode) == model.TCPLength16 {
		size = uint32(binary.BigEndian.Uint16(src[:headerSize]))
	} else {
		size = binary.BigEndian.Uint32(src[:headerSize])
	}
	if maxSize <= 0 {
		maxSize = DefaultMaxFrameSize
	}
	if size > uint32(maxSize) {
		return 0, fmt.Errorf("rawtcp: frame length %d exceeds max %d", size, maxSize)
	}
	return int(size), nil
}

func normalize(mode model.TCPLengthMode) model.TCPLengthMode {
	if mode == "" {
		return model.TCPLength16
	}
	return mode
}
