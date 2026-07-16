//go:build linux

package core

import (
	"encoding/binary"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestUDPReuseportVKeyProgramDispatchesAndDrops(t *testing.T) {
	const socketCount = 4
	fds := make([]int, 0, socketCount)
	defer func() {
		for _, fd := range fds {
			_ = unix.Close(fd)
		}
	}()

	first, err := udpReuseportTestSocket()
	if err != nil {
		t.Fatal(err)
	}
	fds = append(fds, first)
	if err := attachDropAllSocketFilter(first); err != nil {
		t.Fatal(err)
	}
	if err := attachUDPReuseportVKeyProgram(first, []udpVKeySocketRoute{
		{VKey: "alpha", SocketIndex: 1},
		{VKey: "bravo-2", SocketIndex: 2},
	}, 3, 0); err != nil {
		t.Fatal(err)
	}
	addr := &unix.SockaddrInet4{Port: 0, Addr: [4]byte{127, 0, 0, 1}}
	if err := unix.Bind(first, addr); err != nil {
		t.Fatal(err)
	}
	bound, err := unix.Getsockname(first)
	if err != nil {
		t.Fatal(err)
	}
	port := bound.(*unix.SockaddrInet4).Port
	for len(fds) < socketCount {
		fd, socketErr := udpReuseportTestSocket()
		if socketErr != nil {
			t.Fatal(socketErr)
		}
		fds = append(fds, fd)
		if err := unix.Bind(fd, &unix.SockaddrInet4{Port: port, Addr: [4]byte{127, 0, 0, 1}}); err != nil {
			t.Fatal(err)
		}
	}

	sender, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(sender)
	target := &unix.SockaddrInet4{Port: port, Addr: [4]byte{127, 0, 0, 1}}

	testCases := []struct {
		name      string
		payload   []byte
		wantIndex int
	}{
		{name: "alpha", payload: vkeyTestPacket("alpha"), wantIndex: 1},
		{name: "bravo", payload: vkeyTestPacket("bravo-2"), wantIndex: 2},
		{name: "no vkey", payload: []byte{0x45, 0, 0, 20}, wantIndex: 3},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			if err := unix.Sendto(sender, tt.payload, 0, target); err != nil {
				t.Fatal(err)
			}
			if got := waitReadableSocket(t, fds, 500*time.Millisecond); got != tt.wantIndex {
				t.Fatalf("packet reached socket %d, want %d", got, tt.wantIndex)
			}
			buffer := make([]byte, 128)
			if _, _, err := unix.Recvfrom(fds[tt.wantIndex], buffer, 0); err != nil {
				t.Fatal(err)
			}
		})
	}

	if err := unix.Sendto(sender, vkeyTestPacket("unknown"), 0, target); err != nil {
		t.Fatal(err)
	}
	if got := waitReadableSocket(t, fds, 100*time.Millisecond); got != -1 {
		t.Fatalf("unknown vKey reached socket %d, want kernel drop", got)
	}
}

func udpReuseportTestSocket() (int, error) {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC|unix.SOCK_NONBLOCK, 0)
	if err != nil {
		return -1, err
	}
	if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEPORT, 1); err != nil {
		unix.Close(fd)
		return -1, err
	}
	return fd, nil
}

func vkeyTestPacket(key string) []byte {
	out := make([]byte, 8+len(key)+4)
	copy(out, "TXV1")
	binary.BigEndian.PutUint16(out[4:6], uint16(len(key)))
	copy(out[8:], key)
	copy(out[8+len(key):], []byte{0x45, 0, 0, 20})
	return out
}

func waitReadableSocket(t *testing.T, fds []int, timeout time.Duration) int {
	t.Helper()
	poll := make([]unix.PollFd, len(fds))
	for index, fd := range fds {
		poll[index] = unix.PollFd{Fd: int32(fd), Events: unix.POLLIN}
	}
	ready, err := unix.Poll(poll, int(timeout.Milliseconds()))
	if err != nil {
		t.Fatal(err)
	}
	if ready == 0 {
		return -1
	}
	for index := range poll {
		if poll[index].Revents&unix.POLLIN != 0 {
			return index
		}
	}
	return -1
}
