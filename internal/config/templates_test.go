package config

import (
	"testing"

	"tapx/internal/model"
)

func TestBuildRawPairTemplateUDP(t *testing.T) {
	template, err := BuildRawPairTemplate(RawPairTemplateOptions{
		Transport: model.TransportUDP,
		HostA:     "192.0.2.10",
		HostB:     "192.0.2.20",
		Port:      46000,
		VKey:      "lab-key",
	})
	if err != nil {
		t.Fatalf("BuildRawPairTemplate() error = %v", err)
	}
	if len(template.A.Listeners) != 1 || len(template.B.Listeners) != 1 {
		t.Fatalf("udp template listeners = %d/%d, want one each", len(template.A.Listeners), len(template.B.Listeners))
	}
	if got := template.A.Listeners[0].RawUDP.FixedPeer; got != "192.0.2.20:46000" {
		t.Fatalf("side A fixed peer = %q, want host B", got)
	}
	if got := template.B.Listeners[0].RawUDP.FixedPeer; got != "192.0.2.10:46000" {
		t.Fatalf("side B fixed peer = %q, want host A", got)
	}
	if len(template.A.VKeys) != 1 || template.A.Routes[0].VKeyID != "raw-vkey" {
		t.Fatalf("side A vkey/route = %+v %+v, want vKey route binding", template.A.VKeys, template.A.Routes)
	}
	if len(template.RuntimeA.UDPPipes) != 1 || len(template.RuntimeB.UDPPipes) != 1 {
		t.Fatalf("runtime udp pipes = %d/%d, want one each", len(template.RuntimeA.UDPPipes), len(template.RuntimeB.UDPPipes))
	}
	if got := template.A.Devices[0].MTU; got != 1500 {
		t.Fatalf("default device MTU = %d, want 1500", got)
	}
}

func TestBuildRawPairTemplateTCP(t *testing.T) {
	template, err := BuildRawPairTemplate(RawPairTemplateOptions{
		Transport: model.TransportTCP,
		HostA:     "192.0.2.10",
		HostB:     "192.0.2.20",
		Port:      46001,
	})
	if err != nil {
		t.Fatalf("BuildRawPairTemplate() error = %v", err)
	}
	if len(template.A.Listeners) != 1 || len(template.B.Connectors) != 1 {
		t.Fatalf("tcp template endpoints = listeners:%d connectors:%d, want listener/connector", len(template.A.Listeners), len(template.B.Connectors))
	}
	if got := template.B.Connectors[0].Remote; got != "192.0.2.10" {
		t.Fatalf("side B connector remote = %q, want host A", got)
	}
	if len(template.RuntimeA.TCPPipes) != 1 || len(template.RuntimeB.TCPPipes) != 1 {
		t.Fatalf("runtime tcp pipes = %d/%d, want one each", len(template.RuntimeA.TCPPipes), len(template.RuntimeB.TCPPipes))
	}
}

func TestBuildRawPairTemplateRejectsMissingHosts(t *testing.T) {
	if _, err := BuildRawPairTemplate(RawPairTemplateOptions{Transport: model.TransportUDP}); err == nil {
		t.Fatal("BuildRawPairTemplate() error = nil, want missing hosts error")
	}
}
