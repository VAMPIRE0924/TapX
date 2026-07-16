package linkdiag

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

const (
	streamMagic       = "TXDIAG1\n"
	streamVersion     = 1
	maxCredentialSize = 1024
	maxChunkSize      = 256 << 10
	defaultChunkSize  = 64 << 10
	maxDuration       = 10 * time.Second
)

type operation uint8

const (
	opPing operation = iota + 1
	opUpload
	opDownload
	opFrameProbe
)

type Result struct {
	Delay         time.Duration
	UploadBytes   uint64
	DownloadBytes uint64
	UploadBPS     uint64
	DownloadBPS   uint64
	Duration      time.Duration
}

type StreamHelloInspection struct {
	Matched    bool
	Complete   bool
	Required   int
	Credential string
}

// InspectStreamHelloPrefix recognizes a diagnostic hello without consuming
// the stream. Callers use Required to decide how many bytes must be peeked.
func InspectStreamHelloPrefix(payload []byte) (StreamHelloInspection, error) {
	magic := []byte(streamMagic)
	if len(payload) < len(magic) {
		return StreamHelloInspection{
			Matched: bytes.HasPrefix(magic, payload), Required: len(magic),
		}, nil
	}
	if !bytes.Equal(payload[:len(magic)], magic) {
		return StreamHelloInspection{}, nil
	}
	inspection := StreamHelloInspection{Matched: true, Required: len(magic) + 3}
	if len(payload) < inspection.Required {
		return inspection, nil
	}
	if payload[len(magic)] != streamVersion {
		return inspection, fmt.Errorf("unsupported diagnostic version %d", payload[len(magic)])
	}
	credentialSize := int(binary.BigEndian.Uint16(payload[len(magic)+1 : len(magic)+3]))
	if credentialSize > maxCredentialSize {
		return inspection, fmt.Errorf("diagnostic credential exceeds %d bytes", maxCredentialSize)
	}
	inspection.Required += credentialSize
	if len(payload) < inspection.Required {
		return inspection, nil
	}
	inspection.Complete = true
	inspection.Credential = string(payload[len(magic)+3 : inspection.Required])
	return inspection, nil
}

// ServeStream handles diagnostics on a stream dedicated to TapX control
// traffic. It never reads from or writes to a TUN/TAP device.
func ServeStream(ctx context.Context, conn net.Conn, credential string) error {
	if conn == nil {
		return errors.New("diagnostic stream is required")
	}
	defer conn.Close()
	if err := applyDeadline(ctx, conn); err != nil {
		return err
	}
	reader := bufio.NewReaderSize(conn, maxChunkSize+4)
	provided, err := readHello(reader)
	if err != nil {
		return err
	}
	if provided != credential {
		return errors.New("diagnostic credential rejected")
	}
	if err := writeAll(conn, []byte{streamVersion}); err != nil {
		return err
	}
	buffer := make([]byte, maxChunkSize)
	for {
		op, duration, chunkSize, err := readCommand(reader)
		if err != nil {
			return err
		}
		switch op {
		case opPing:
			if err := writeAll(conn, []byte{byte(opPing)}); err != nil {
				return err
			}
		case opUpload:
			if err := writeAll(conn, []byte{byte(opUpload)}); err != nil {
				return err
			}
			received, err := receiveChunks(reader, buffer)
			if err != nil {
				return err
			}
			if err := writeUint64(conn, received); err != nil {
				return err
			}
		case opDownload:
			if chunkSize <= 0 || chunkSize > maxChunkSize {
				return fmt.Errorf("invalid diagnostic chunk size %d", chunkSize)
			}
			if duration <= 0 || duration > maxDuration {
				return fmt.Errorf("invalid diagnostic duration %s", duration)
			}
			if _, err := rand.Read(buffer[:chunkSize]); err != nil {
				return err
			}
			if err := writeAll(conn, []byte{byte(opDownload)}); err != nil {
				return err
			}
			if _, err := sendChunks(conn, buffer[:chunkSize], duration); err != nil {
				return err
			}
		case opFrameProbe:
			if chunkSize <= 0 || chunkSize > maxChunkSize {
				return fmt.Errorf("invalid frame probe size %d", chunkSize)
			}
			if err := writeAll(conn, []byte{byte(opFrameProbe)}); err != nil {
				return err
			}
			if _, err := io.ReadFull(reader, buffer[:chunkSize]); err != nil {
				return err
			}
			digest := sha256.Sum256(buffer[:chunkSize])
			if err := writeAll(conn, digest[:]); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported diagnostic operation %d", op)
		}
	}
}

// ProbeFrame confirms that a full logical TUN/TAP frame can cross the stream
// and be reassembled intact. It is a stream-capacity check, not a UDP PMTU
// measurement; outer stream segmentation remains managed by the transport.
func ProbeFrame(ctx context.Context, conn net.Conn, credential string, size int) (time.Duration, error) {
	if size <= 0 || size > maxChunkSize {
		return 0, fmt.Errorf("frame probe size %d is outside 1..%d", size, maxChunkSize)
	}
	reader, err := startClient(ctx, conn, credential)
	if err != nil {
		return 0, err
	}
	payload := make([]byte, size)
	if _, err := rand.Read(payload); err != nil {
		return 0, err
	}
	want := sha256.Sum256(payload)
	started := time.Now()
	if err := writeCommand(conn, opFrameProbe, 0, size); err != nil {
		return 0, err
	}
	if err := expectOperation(reader, opFrameProbe); err != nil {
		return 0, err
	}
	if err := writeAll(conn, payload); err != nil {
		return 0, err
	}
	var got [sha256.Size]byte
	if _, err := io.ReadFull(reader, got[:]); err != nil {
		return 0, err
	}
	if got != want {
		return 0, errors.New("frame probe digest mismatch")
	}
	return time.Since(started), nil
}

func Ping(ctx context.Context, conn net.Conn, credential string) (time.Duration, error) {
	reader, err := startClient(ctx, conn, credential)
	if err != nil {
		return 0, err
	}
	started := time.Now()
	if err := writeCommand(conn, opPing, 0, 0); err != nil {
		return 0, err
	}
	if err := expectOperation(reader, opPing); err != nil {
		return 0, err
	}
	return time.Since(started), nil
}

func Throughput(ctx context.Context, conn net.Conn, credential string, duration time.Duration) (Result, error) {
	return ThroughputWithChunkSize(ctx, conn, credential, duration, defaultChunkSize)
}

func ThroughputWithChunkSize(ctx context.Context, conn net.Conn, credential string, duration time.Duration, chunkSize int) (Result, error) {
	if duration <= 0 {
		duration = 2 * time.Second
	}
	if duration > maxDuration {
		duration = maxDuration
	}
	if chunkSize <= 0 || chunkSize > maxChunkSize {
		return Result{}, fmt.Errorf("diagnostic chunk size %d is outside 1..%d", chunkSize, maxChunkSize)
	}
	reader, err := startClient(ctx, conn, credential)
	if err != nil {
		return Result{}, err
	}
	result := Result{Duration: duration}
	payload := make([]byte, chunkSize)
	if _, err := rand.Read(payload); err != nil {
		return Result{}, err
	}

	if err := writeCommand(conn, opUpload, duration, len(payload)); err != nil {
		return Result{}, err
	}
	if err := expectOperation(reader, opUpload); err != nil {
		return Result{}, err
	}
	uploadStarted := time.Now()
	uploadBytes, err := sendChunks(conn, payload, duration)
	if err != nil {
		return Result{}, err
	}
	acknowledged, err := readUint64(reader)
	if err != nil {
		return Result{}, err
	}
	uploadElapsed := time.Since(uploadStarted)
	if acknowledged < uploadBytes {
		uploadBytes = acknowledged
	}

	if err := writeCommand(conn, opDownload, duration, len(payload)); err != nil {
		return Result{}, err
	}
	if err := expectOperation(reader, opDownload); err != nil {
		return Result{}, err
	}
	downloadStarted := time.Now()
	downloadBytes, err := receiveChunks(reader, make([]byte, maxChunkSize))
	if err != nil {
		return Result{}, err
	}
	downloadElapsed := time.Since(downloadStarted)

	result.UploadBytes = uploadBytes
	result.DownloadBytes = downloadBytes
	result.UploadBPS = bitsPerSecond(uploadBytes, uploadElapsed)
	result.DownloadBPS = bitsPerSecond(downloadBytes, downloadElapsed)
	return result, nil
}

func startClient(ctx context.Context, conn net.Conn, credential string) (*bufio.Reader, error) {
	if conn == nil {
		return nil, errors.New("diagnostic stream is required")
	}
	if len(credential) > maxCredentialSize {
		return nil, fmt.Errorf("diagnostic credential exceeds %d bytes", maxCredentialSize)
	}
	if err := applyDeadline(ctx, conn); err != nil {
		return nil, err
	}
	hello := make([]byte, len(streamMagic)+3+len(credential))
	copy(hello, streamMagic)
	hello[len(streamMagic)] = streamVersion
	binary.BigEndian.PutUint16(hello[len(streamMagic)+1:], uint16(len(credential)))
	copy(hello[len(streamMagic)+3:], credential)
	if err := writeAll(conn, hello); err != nil {
		return nil, err
	}
	reader := bufio.NewReaderSize(conn, maxChunkSize+4)
	version, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	if version != streamVersion {
		return nil, fmt.Errorf("unsupported diagnostic version %d", version)
	}
	return reader, nil
}

func readHello(reader *bufio.Reader) (string, error) {
	magic := make([]byte, len(streamMagic))
	if _, err := io.ReadFull(reader, magic); err != nil {
		return "", err
	}
	if string(magic) != streamMagic {
		return "", errors.New("diagnostic stream magic mismatch")
	}
	version, err := reader.ReadByte()
	if err != nil {
		return "", err
	}
	if version != streamVersion {
		return "", fmt.Errorf("unsupported diagnostic version %d", version)
	}
	var sizeBytes [2]byte
	if _, err := io.ReadFull(reader, sizeBytes[:]); err != nil {
		return "", err
	}
	size := int(binary.BigEndian.Uint16(sizeBytes[:]))
	if size > maxCredentialSize {
		return "", fmt.Errorf("diagnostic credential exceeds %d bytes", maxCredentialSize)
	}
	credential := make([]byte, size)
	if _, err := io.ReadFull(reader, credential); err != nil {
		return "", err
	}
	return string(credential), nil
}

func writeCommand(writer io.Writer, op operation, duration time.Duration, chunkSize int) error {
	var command [9]byte
	command[0] = byte(op)
	binary.BigEndian.PutUint32(command[1:5], uint32(duration/time.Millisecond))
	binary.BigEndian.PutUint32(command[5:9], uint32(chunkSize))
	return writeAll(writer, command[:])
}

func readCommand(reader io.Reader) (operation, time.Duration, int, error) {
	var command [9]byte
	if _, err := io.ReadFull(reader, command[:]); err != nil {
		return 0, 0, 0, err
	}
	return operation(command[0]), time.Duration(binary.BigEndian.Uint32(command[1:5])) * time.Millisecond,
		int(binary.BigEndian.Uint32(command[5:9])), nil
}

func expectOperation(reader *bufio.Reader, want operation) error {
	got, err := reader.ReadByte()
	if err != nil {
		return err
	}
	if operation(got) != want {
		return fmt.Errorf("diagnostic response %d does not match request %d", got, want)
	}
	return nil
}

func sendChunks(writer io.Writer, payload []byte, duration time.Duration) (uint64, error) {
	deadline := time.Now().Add(duration)
	var total uint64
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
	for time.Now().Before(deadline) {
		if err := writeAll(writer, header[:]); err != nil {
			return total, err
		}
		if err := writeAll(writer, payload); err != nil {
			return total, err
		}
		total += uint64(len(payload))
	}
	clear(header[:])
	if err := writeAll(writer, header[:]); err != nil {
		return total, err
	}
	return total, nil
}

func receiveChunks(reader io.Reader, buffer []byte) (uint64, error) {
	var total uint64
	var header [4]byte
	for {
		if _, err := io.ReadFull(reader, header[:]); err != nil {
			return total, err
		}
		size := int(binary.BigEndian.Uint32(header[:]))
		if size == 0 {
			return total, nil
		}
		if size > len(buffer) || size > maxChunkSize {
			return total, fmt.Errorf("diagnostic chunk size %d exceeds limit", size)
		}
		if _, err := io.ReadFull(reader, buffer[:size]); err != nil {
			return total, err
		}
		total += uint64(size)
	}
}

func applyDeadline(ctx context.Context, conn net.Conn) error {
	deadline := time.Now().Add(30 * time.Second)
	if contextDeadline, ok := ctx.Deadline(); ok {
		deadline = contextDeadline
	}
	return conn.SetDeadline(deadline)
}

func writeUint64(writer io.Writer, value uint64) error {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	return writeAll(writer, encoded[:])
}

func writeAll(writer io.Writer, payload []byte) error {
	for len(payload) > 0 {
		n, err := writer.Write(payload)
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

func readUint64(reader io.Reader) (uint64, error) {
	var encoded [8]byte
	if _, err := io.ReadFull(reader, encoded[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(encoded[:]), nil
}

func bitsPerSecond(bytes uint64, elapsed time.Duration) uint64 {
	if elapsed <= 0 {
		return 0
	}
	return uint64(float64(bytes*8) / elapsed.Seconds())
}
