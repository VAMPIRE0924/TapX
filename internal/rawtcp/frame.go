package rawtcp

import (
	"encoding/binary"
	"fmt"
	"io"

	"tapx/internal/model"
)

const DefaultMaxFrameSize = 65535

func WriteFrame(w io.Writer, mode model.TCPLengthMode, payload []byte) error {
	switch normalize(mode) {
	case model.TCPLength16:
		if len(payload) > 65535 {
			return fmt.Errorf("rawtcp: frame length %d exceeds uint16", len(payload))
		}
		var header [2]byte
		binary.BigEndian.PutUint16(header[:], uint16(len(payload)))
		if _, err := w.Write(header[:]); err != nil {
			return err
		}
	case model.TCPLength32:
		var header [4]byte
		binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
		if _, err := w.Write(header[:]); err != nil {
			return err
		}
	default:
		return fmt.Errorf("rawtcp: unsupported length mode %q", mode)
	}
	_, err := w.Write(payload)
	return err
}

func ReadFrame(r io.Reader, mode model.TCPLengthMode, maxSize int) ([]byte, error) {
	if maxSize <= 0 {
		maxSize = DefaultMaxFrameSize
	}

	var size uint32
	switch normalize(mode) {
	case model.TCPLength16:
		var header [2]byte
		if _, err := io.ReadFull(r, header[:]); err != nil {
			return nil, err
		}
		size = uint32(binary.BigEndian.Uint16(header[:]))
	case model.TCPLength32:
		var header [4]byte
		if _, err := io.ReadFull(r, header[:]); err != nil {
			return nil, err
		}
		size = binary.BigEndian.Uint32(header[:])
	default:
		return nil, fmt.Errorf("rawtcp: unsupported length mode %q", mode)
	}

	if size > uint32(maxSize) {
		return nil, fmt.Errorf("rawtcp: frame length %d exceeds max %d", size, maxSize)
	}
	payload := make([]byte, size)
	_, err := io.ReadFull(r, payload)
	return payload, err
}

func normalize(mode model.TCPLengthMode) model.TCPLengthMode {
	if mode == "" {
		return model.TCPLength16
	}
	return mode
}
