package panel

import (
	"runtime"
	"sync"
	"time"
)

type SystemDiagnostic struct {
	CPUPercent     float64 `json:"cpuPercent"`
	CPUCores       int     `json:"cpuCores"`
	MemoryUsed     uint64  `json:"memoryUsed"`
	MemoryTotal    uint64  `json:"memoryTotal"`
	SwapUsed       uint64  `json:"swapUsed"`
	SwapTotal      uint64  `json:"swapTotal"`
	StorageUsed    uint64  `json:"storageUsed"`
	StorageTotal   uint64  `json:"storageTotal"`
	RunningPipes   int     `json:"runningPipes"`
	DropCount      uint64  `json:"dropCount"`
	TCPConnections int     `json:"tcpConnections"`
	UDPConnections int     `json:"udpConnections"`
	DiskReadBPS    uint64  `json:"diskReadBytesPerSecond"`
	DiskWriteBPS   uint64  `json:"diskWriteBytesPerSecond"`
	Load1          float64 `json:"load1"`
	Load5          float64 `json:"load5"`
	Load15         float64 `json:"load15"`
}

type systemSampler struct {
	mu         sync.Mutex
	lastCPU    cpuSample
	lastCPUAt  time.Time
	lastDisk   diskSample
	lastDiskAt time.Time
}

type cpuSample struct {
	total uint64
	idle  uint64
	ok    bool
}

type diskSample struct {
	readBytes  uint64
	writeBytes uint64
	ok         bool
}

func (s *systemSampler) Sample(stats StatsReport, state RuntimeState) SystemDiagnostic {
	out := readSystemDiagnostic()
	out.CPUCores = runtime.NumCPU()
	out.RunningPipes = len(state.UDPPipes) + len(state.TCPPipes) + len(state.XrayPipes)
	out.DropCount = stats.Totals.DropsGuard + stats.Totals.DropsIO
	out.TCPConnections = len(state.TCPPipes)
	out.UDPConnections = len(state.UDPPipes)

	now := time.Now()
	current := readCPUSample()
	disk := readDiskSample()
	s.mu.Lock()
	if current.ok {
		if s.lastCPU.ok && now.After(s.lastCPUAt) {
			totalDelta := current.total - s.lastCPU.total
			idleDelta := current.idle - s.lastCPU.idle
			if totalDelta > 0 && idleDelta <= totalDelta {
				out.CPUPercent = float64(totalDelta-idleDelta) * 100 / float64(totalDelta)
			}
		}
		s.lastCPU = current
		s.lastCPUAt = now
	}
	if disk.ok {
		if s.lastDisk.ok && now.After(s.lastDiskAt) {
			seconds := now.Sub(s.lastDiskAt).Seconds()
			out.DiskReadBPS = counterRate(s.lastDisk.readBytes, disk.readBytes, seconds)
			out.DiskWriteBPS = counterRate(s.lastDisk.writeBytes, disk.writeBytes, seconds)
		}
		s.lastDisk = disk
		s.lastDiskAt = now
	}
	s.mu.Unlock()
	return out
}
