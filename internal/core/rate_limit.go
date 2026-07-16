package core

import (
	"io"
	"math"
	"net"
	"sync"
	"time"

	xbuf "github.com/xtls/xray-core/common/buf"

	"tapx/internal/config"
)

// userTrafficRates maps the user-facing upload/download limits onto the local
// pipe directions. A listener sees the remote user on the network side, while
// a connector sees the user on the local device side.
func userTrafficRates(endpointKind string, binding config.RuntimeBinding) (networkToDevice, deviceToNetwork uint64) {
	switch endpointKind {
	case "listener":
		return binding.UploadRateLimit, binding.DownloadRateLimit
	case "connector":
		return binding.DownloadRateLimit, binding.UploadRateLimit
	default:
		return 0, 0
	}
}

// addressGuardSource selects the address constrained by a precompiled policy.
// Remote identities are on the network side; ordinary device policies are local.
func addressGuardSource(remoteIdentity, deviceToNetwork bool) bool {
	if remoteIdentity {
		return !deviceToNetwork
	}
	return deviceToNetwork
}

func applyUserRateLimits(conn net.Conn, endpointKind string, binding config.RuntimeBinding) net.Conn {
	readRate, writeRate := userTrafficRates(endpointKind, binding)
	if readRate == 0 && writeRate == 0 {
		return conn
	}
	return newRateLimitedConn(conn, readRate, writeRate)
}

type rateLimitedConn struct {
	net.Conn
	readPacer  *bytePacer
	writePacer *bytePacer
	closed     chan struct{}
	closeOnce  sync.Once
}

func newRateLimitedConn(conn net.Conn, readRate, writeRate uint64) net.Conn {
	wrapped := &rateLimitedConn{Conn: conn, closed: make(chan struct{})}
	if readRate > 0 {
		wrapped.readPacer = &bytePacer{bitsPerSecond: readRate}
	}
	if writeRate > 0 {
		wrapped.writePacer = &bytePacer{bitsPerSecond: writeRate}
	}
	return wrapped
}

func (c *rateLimitedConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 && c.readPacer != nil {
		if waitErr := c.readPacer.wait(n, c.closed); waitErr != nil && err == nil {
			err = waitErr
		}
	}
	return n, err
}

func (c *rateLimitedConn) ReadMultiBuffer() (xbuf.MultiBuffer, error) {
	var (
		mb  xbuf.MultiBuffer
		err error
	)
	if reader, ok := c.Conn.(xbuf.Reader); ok {
		mb, err = reader.ReadMultiBuffer()
	} else {
		var buffer *xbuf.Buffer
		buffer, err = xbuf.ReadBuffer(c.Conn)
		if buffer != nil {
			mb = xbuf.MultiBuffer{buffer}
		}
	}
	if size := int(mb.Len()); size > 0 && c.readPacer != nil {
		if waitErr := c.readPacer.wait(size, c.closed); waitErr != nil && err == nil {
			err = waitErr
		}
	}
	return mb, err
}

func (c *rateLimitedConn) Write(p []byte) (int, error) {
	if len(p) > 0 && c.writePacer != nil {
		if err := c.writePacer.wait(len(p), c.closed); err != nil {
			return 0, err
		}
	}
	return c.Conn.Write(p)
}

func (c *rateLimitedConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

type bytePacer struct {
	bitsPerSecond uint64
	next          time.Time
}

func (p *bytePacer) wait(byteCount int, closed <-chan struct{}) error {
	if byteCount <= 0 || p.bitsPerSecond == 0 {
		return nil
	}
	now := time.Now()
	cost := time.Duration(math.Ceil(float64(byteCount) * 8 * float64(time.Second) / float64(p.bitsPerSecond)))
	burst := 50 * time.Millisecond
	if cost > burst {
		burst = cost
	}
	floor := now.Add(-burst)
	if p.next.Before(floor) {
		p.next = floor
	}
	p.next = p.next.Add(cost)
	delay := time.Until(p.next)
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-closed:
		return io.ErrClosedPipe
	}
}
