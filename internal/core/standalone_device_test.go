package core

import (
	"reflect"
	"testing"

	"tapx/internal/config"
)

func TestStandaloneRuntimeDevicesExcludesEveryTransportOwnedDevice(t *testing.T) {
	runtime := &config.GeneratedRuntime{
		Devices:   []config.RuntimeDevice{{ID: "unused"}, {ID: "udp"}, {ID: "tcp"}, {ID: "xray"}},
		UDPPipes:  []config.RuntimeUDPPipe{{DeviceID: "udp"}},
		TCPPipes:  []config.RuntimeTCPPipe{{DeviceID: "tcp"}},
		XrayPipes: []config.RuntimeXrayPipe{{DeviceID: "xray"}},
	}
	got := standaloneRuntimeDevices(runtime)
	if want := []config.RuntimeDevice{{ID: "unused"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("standalone devices = %#v, want %#v", got, want)
	}
}
