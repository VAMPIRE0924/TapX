package netapply

import "testing"

func TestNeedsApply(t *testing.T) {
	tests := []struct {
		name string
		cfg  DeviceConfig
		want bool
	}{
		{name: "empty", cfg: DeviceConfig{}, want: false},
		{name: "mtu", cfg: DeviceConfig{MTU: 1500}, want: true},
		{name: "automatic link optimization", cfg: DeviceConfig{LinkAutoOptimize: true}, want: true},
		{name: "bridge", cfg: DeviceConfig{Bridge: BridgeConfig{Enabled: true}}, want: true},
		{name: "route", cfg: DeviceConfig{Routes: []RouteConfig{{Enabled: true}}}, want: true},
		{name: "disabled route", cfg: DeviceConfig{Routes: []RouteConfig{{Enabled: false}}}, want: false},
		{name: "dns", cfg: DeviceConfig{DNS: DNSConfig{Enabled: true}}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := needsApply(test.cfg); got != test.want {
				t.Fatalf("needsApply() = %t, want %t", got, test.want)
			}
		})
	}
}
