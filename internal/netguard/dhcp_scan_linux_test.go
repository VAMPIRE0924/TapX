//go:build linux

package netguard

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanDHCPFileFormats(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name string
		body string
	}{
		{"dnsmasq.conf", "dhcp-range=set:lan,192.0.2.10,192.0.2.20,255.255.255.0,12h\n"},
		{"dhcpd.conf", "subnet 198.51.100.0 netmask 255.255.255.0 {\n range 198.51.100.10 198.51.100.20;\n}\n"},
		{"kea-dhcp4.conf", `{"Dhcp4":{"subnet4":[{"pools":[{"pool":"203.0.113.10 - 203.0.113.20"}]}]}}`},
	}
	for _, test := range cases {
		path := filepath.Join(dir, test.name)
		if err := os.WriteFile(path, []byte(test.body), 0o600); err != nil {
			t.Fatal(err)
		}
		pools := scanDHCPFile(path)
		if len(pools) != 1 || !pools[0].start.IsValid() || !pools[0].end.IsValid() {
			t.Fatalf("%s pools = %#v", test.name, pools)
		}
	}
}
