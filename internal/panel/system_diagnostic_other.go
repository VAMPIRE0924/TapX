//go:build !linux

package panel

import goruntime "runtime"

func readSystemDiagnostic() SystemDiagnostic {
	var mem goruntime.MemStats
	goruntime.ReadMemStats(&mem)
	return SystemDiagnostic{
		MemoryUsed:  mem.HeapAlloc,
		MemoryTotal: mem.HeapSys,
	}
}

func readCPUSample() cpuSample {
	return cpuSample{}
}

func readDiskSample() diskSample {
	return diskSample{}
}
