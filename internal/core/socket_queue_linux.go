//go:build linux

package core

const maxAutomaticSocketQueueBytes = 64 << 20

// socketQueueBytes translates the UI's frame-count queue into a bounded
// kernel socket queue. The kernel remains the queue owner, so no Go goroutine
// or per-packet handoff is introduced into the data path.
func socketQueueBytes(frames, maxFrameSize int) int {
	if frames <= 0 || maxFrameSize <= 0 {
		return 0
	}
	const framingAllowance = 64
	bytesPerFrame := maxFrameSize + framingAllowance
	if frames > maxAutomaticSocketQueueBytes/bytesPerFrame {
		return maxAutomaticSocketQueueBytes
	}
	return frames * bytesPerFrame
}
