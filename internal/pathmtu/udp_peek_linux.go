//go:build linux

package pathmtu

import (
	"errors"
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// peekUDPControlDatagram reports whether the next datagram is a TapX path
// probe without consuming it. This keeps early data-plane frames queued while
// the probe socket is being handed to the C worker.
func peekUDPControlDatagram(conn *net.UDPConn, buffer, prefix []byte) (bool, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return false, err
	}
	var n int
	var recvErr error
	if err := raw.Read(func(fd uintptr) bool {
		n, _, recvErr = unix.Recvfrom(int(fd), buffer, unix.MSG_PEEK|unix.MSG_DONTWAIT)
		if errors.Is(recvErr, unix.EAGAIN) || errors.Is(recvErr, unix.EWOULDBLOCK) {
			recvErr = nil
			return false
		}
		return true
	}); err != nil {
		return false, err
	}
	if recvErr != nil {
		return false, recvErr
	}
	if n <= 0 {
		return false, fmt.Errorf("peek UDP datagram returned %d bytes", n)
	}
	packet, ok := stripUDPPacketPrefix(buffer[:n], prefix)
	if !ok {
		return false, nil
	}
	_, err = ParseProbe(packet)
	return err == nil, nil
}
