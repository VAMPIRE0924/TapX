//go:build linux

package core

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"tapx/internal/model"
	"tapx/internal/rawtcp"
)

func TestInspectTCPDispatchPrefix(t *testing.T) {
	plainPayload := append([]byte{0x45}, make([]byte, 19)...)
	plainWire := tcpDispatchTestFrame(model.TCPLength16, plainPayload)
	plain, complete, _, err := inspectTCPDispatchPrefix(plainWire, model.TCPLength16, 1400)
	if err != nil || !complete || plain.Diagnostic || plain.VKey != "" {
		t.Fatalf("plain probe = %+v, complete=%v, err=%v", plain, complete, err)
	}

	vkeyPayload := make([]byte, rawVKeyHeaderBaseSize+len("alpha")+20)
	writeRawVKeyHeader(vkeyPayload, []byte("alpha"))
	vkeyWire := tcpDispatchTestFrame(model.TCPLength16, vkeyPayload)
	vkey, complete, _, err := inspectTCPDispatchPrefix(vkeyWire, model.TCPLength16, 1400)
	if err != nil || !complete || vkey.Diagnostic || vkey.VKey != "alpha" {
		t.Fatalf("vKey probe = %+v, complete=%v, err=%v", vkey, complete, err)
	}

	diagnosticWire := append([]byte("TXDIAG1\n"), 1, 0, 5)
	diagnosticWire = append(diagnosticWire, "alpha"...)
	diagnostic, complete, _, err := inspectTCPDispatchPrefix(diagnosticWire, model.TCPLength16, 1400)
	if err != nil || !complete || !diagnostic.Diagnostic || diagnostic.VKey != "alpha" {
		t.Fatalf("diagnostic probe = %+v, complete=%v, err=%v", diagnostic, complete, err)
	}

	_, complete, required, err := inspectTCPDispatchPrefix(vkeyWire[:6], model.TCPLength16, 1400)
	if err != nil || complete || required <= 6 {
		t.Fatalf("partial probe complete=%v required=%d err=%v", complete, required, err)
	}
}

func TestPeekTCPDispatchDoesNotConsumeFragmentedFirstFrame(t *testing.T) {
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	payload := make([]byte, rawVKeyHeaderBaseSize+len("bravo")+64)
	writeRawVKeyHeader(payload, []byte("bravo"))
	wire := tcpDispatchTestFrame(model.TCPLength32, payload)
	clientErr := make(chan error, 1)
	go func() {
		conn, err := net.DialTCP("tcp", nil, listener.Addr().(*net.TCPAddr))
		if err != nil {
			clientErr <- err
			return
		}
		defer conn.Close()
		for _, chunk := range [][]byte{wire[:1], wire[1:7], wire[7:]} {
			if _, err := conn.Write(chunk); err != nil {
				clientErr <- err
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
		clientErr <- nil
	}()

	conn, err := listener.AcceptTCP()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	probe, err := peekTCPDispatch(conn, model.TCPLength32, 1400, time.Second)
	if err != nil || probe.Diagnostic || probe.VKey != "bravo" {
		t.Fatalf("peek probe = %+v, err=%v", probe, err)
	}
	got := make([]byte, len(wire))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, wire) {
		t.Fatal("MSG_PEEK consumed or changed the first frame")
	}
	if err := <-clientErr; err != nil {
		t.Fatal(err)
	}
}

func tcpDispatchTestFrame(mode model.TCPLengthMode, payload []byte) []byte {
	headerSize, _ := rawtcp.FrameHeaderSize(mode)
	wire := make([]byte, headerSize+len(payload))
	if mode == model.TCPLength32 {
		binary.BigEndian.PutUint32(wire, uint32(len(payload)))
	} else {
		binary.BigEndian.PutUint16(wire, uint16(len(payload)))
	}
	copy(wire[headerSize:], payload)
	return wire
}
