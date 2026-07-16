//go:build linux

package panel

import (
	"math"
	"testing"
)

func TestStatfsUsage(t *testing.T) {
	total, used, ok := statfsUsage(100, 25, 4096)
	if !ok || total != 409600 || used != 307200 {
		t.Fatalf("statfsUsage() = (%d, %d, %t)", total, used, ok)
	}

	invalid := []struct {
		blocks    uint64
		available uint64
		blockSize int64
	}{
		{blocks: 100, available: 25, blockSize: 0},
		{blocks: 100, available: 25, blockSize: -1},
		{blocks: 100, available: 101, blockSize: 4096},
		{blocks: math.MaxUint64, available: 0, blockSize: 2},
	}
	for _, test := range invalid {
		if _, _, ok := statfsUsage(test.blocks, test.available, test.blockSize); ok {
			t.Fatalf("statfsUsage(%d, %d, %d) accepted invalid values", test.blocks, test.available, test.blockSize)
		}
	}
}

func TestParseDiskStatsCountsWholeDevicesOnly(t *testing.T) {
	raw := []byte("8 0 sda 1 0 10 0 2 0 20 0 0 0 0 0 0 0 0\n" +
		"8 1 sda1 1 0 100 0 2 0 200 0 0 0 0 0 0 0 0\n" +
		"259 0 nvme0n1 1 0 30 0 2 0 40 0 0 0 0 0 0 0 0\n")
	sample := parseDiskStats(raw, func(name string) bool { return name == "sda" || name == "nvme0n1" })
	if !sample.ok || sample.readBytes != 40*512 || sample.writeBytes != 60*512 {
		t.Fatalf("disk sample = %+v", sample)
	}
}

func TestParseLoadAverage(t *testing.T) {
	load1, load5, load15 := parseLoadAverage([]byte("1.25 0.75 0.50 2/100 123\n"))
	if load1 != 1.25 || load5 != 0.75 || load15 != 0.5 {
		t.Fatalf("load average = %v %v %v", load1, load5, load15)
	}
}
