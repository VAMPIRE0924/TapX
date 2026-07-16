//go:build !linux

package pathmtu

import "net"

// TapX data-plane workers are Linux-only. Other platforms keep the portable
// control-plane behavior so configuration and unit tests remain buildable.
func peekUDPControlDatagram(_ *net.UDPConn, _, _ []byte) (bool, error) {
	return true, nil
}
