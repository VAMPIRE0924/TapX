//go:build linux

package panel

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func readSystemDiagnostic() SystemDiagnostic {
	out := SystemDiagnostic{}
	if mem, err := readMemInfo(); err == nil {
		out.MemoryTotal = mem["MemTotal"]
		if available := mem["MemAvailable"]; available > 0 && out.MemoryTotal >= available {
			out.MemoryUsed = out.MemoryTotal - available
		}
		out.SwapTotal = mem["SwapTotal"]
		if free := mem["SwapFree"]; free > 0 && out.SwapTotal >= free {
			out.SwapUsed = out.SwapTotal - free
		}
	}
	var fs unix.Statfs_t
	if err := unix.Statfs("/", &fs); err == nil {
		if total, used, ok := statfsUsage(uint64(fs.Blocks), uint64(fs.Bavail), fs.Bsize); ok {
			out.StorageTotal = total
			out.StorageUsed = used
		}
	}
	if load, err := os.ReadFile("/proc/loadavg"); err == nil {
		out.Load1, out.Load5, out.Load15 = parseLoadAverage(load)
	}
	return out
}

func readDiskSample() diskSample {
	raw, err := os.ReadFile("/proc/diskstats")
	if err != nil {
		return diskSample{}
	}
	return parseDiskStats(raw, wholeBlockDevice)
}

func wholeBlockDevice(name string) bool {
	if name == "" || strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") || strings.HasPrefix(name, "zram") {
		return false
	}
	_, err := os.Stat(filepath.Join("/sys/class/block", name, "partition"))
	return os.IsNotExist(err)
}

func parseDiskStats(raw []byte, include func(string) bool) diskSample {
	const sectorBytes = uint64(512)
	out := diskSample{}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 10 || !include(fields[2]) {
			continue
		}
		readSectors, readErr := strconv.ParseUint(fields[5], 10, 64)
		writeSectors, writeErr := strconv.ParseUint(fields[9], 10, 64)
		if readErr != nil || writeErr != nil || readSectors > math.MaxUint64/sectorBytes || writeSectors > math.MaxUint64/sectorBytes {
			continue
		}
		readBytes := readSectors * sectorBytes
		writeBytes := writeSectors * sectorBytes
		if out.readBytes > math.MaxUint64-readBytes || out.writeBytes > math.MaxUint64-writeBytes {
			continue
		}
		out.readBytes += readBytes
		out.writeBytes += writeBytes
		out.ok = true
	}
	return out
}

func parseLoadAverage(raw []byte) (float64, float64, float64) {
	fields := strings.Fields(string(raw))
	if len(fields) < 3 {
		return 0, 0, 0
	}
	load1, _ := strconv.ParseFloat(fields[0], 64)
	load5, _ := strconv.ParseFloat(fields[1], 64)
	load15, _ := strconv.ParseFloat(fields[2], 64)
	return load1, load5, load15
}

func statfsUsage(blocks, available uint64, blockSize int64) (uint64, uint64, bool) {
	if blockSize <= 0 {
		return 0, 0, false
	}
	size := uint64(blockSize)
	if blocks > math.MaxUint64/size || available > blocks || available > math.MaxUint64/size {
		return 0, 0, false
	}
	total := blocks * size
	return total, total - available*size, true
}

func readCPUSample() cpuSample {
	raw, err := os.ReadFile("/proc/stat")
	if err != nil {
		return cpuSample{}
	}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] != "cpu" {
			continue
		}
		var total uint64
		for _, value := range fields[1:] {
			n, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return cpuSample{}
			}
			total += n
		}
		idle, _ := strconv.ParseUint(fields[4], 10, 64)
		if len(fields) > 5 {
			if iowait, err := strconv.ParseUint(fields[5], 10, 64); err == nil {
				idle += iowait
			}
		}
		return cpuSample{total: total, idle: idle, ok: true}
	}
	return cpuSample{}
}

func readMemInfo() (map[string]uint64, error) {
	raw, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return nil, err
	}
	out := map[string]uint64{}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		out[key] = value * 1024
	}
	return out, nil
}
