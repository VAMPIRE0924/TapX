//go:build linux

package pathmtu

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestUDPExchangerHandoffLeavesFirstDataDatagramQueued(t *testing.T) {
	server := listenUDP4(t)
	defer server.Close()
	client := listenUDP4(t)
	defer client.Close()

	payload := []byte("tapx-data-plane-frame")
	if _, err := server.WriteToUDP(payload, client.LocalAddr().(*net.UDPAddr)); err != nil {
		t.Fatal(err)
	}

	exchanger := &UDPExchanger{
		Conn: client, Peer: server.LocalAddr().(*net.UDPAddr), Timeout: time.Second,
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := exchanger.Handoff(ctx, time.Second); err != nil {
		t.Fatal(err)
	}

	if err := client.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 64)
	n, peer, err := client.ReadFromUDP(buffer)
	if err != nil {
		t.Fatal(err)
	}
	if !sameUDPAddr(peer, server.LocalAddr().(*net.UDPAddr)) || string(buffer[:n]) != string(payload) {
		t.Fatalf("queued datagram = %q from %v", buffer[:n], peer)
	}
}
