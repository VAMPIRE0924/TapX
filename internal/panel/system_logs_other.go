//go:build !linux

package panel

import "context"

func readSystemLogs(context.Context, int) []LogEvent {
	return nil
}
