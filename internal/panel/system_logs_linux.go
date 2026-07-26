//go:build linux

package panel

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func readSystemLogs(parent context.Context, limit int) []LogEvent {
	if limit <= 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()

	commands := [][]string{
		{"logread", "-e", "tapx"},
		{"journalctl", "--no-pager", "-n", strconv.Itoa(limit), "-o", "short-iso", "-u", "tapx-panel.service"},
	}
	for _, command := range commands {
		output, err := exec.CommandContext(ctx, command[0], command[1:]...).Output()
		if err != nil || len(strings.TrimSpace(string(output))) == 0 {
			continue
		}
		return systemLogEvents(string(output), limit)
	}
	return nil
}

func systemLogEvents(output string, limit int) []LogEvent {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	events := make([]LogEvent, 0, min(limit, len(lines)))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		events = append(events, LogEvent{
			Seq:     uint64(len(events) + 1),
			Time:    "",
			Level:   inferSystemLogLevel(line),
			Action:  "syslog",
			Message: line,
		})
	}
	if len(events) > limit {
		events = events[len(events)-limit:]
	}
	return events
}

func inferSystemLogLevel(line string) string {
	normalized := strings.ToLower(line)
	switch {
	case strings.Contains(normalized, "panic"), strings.Contains(normalized, "fatal"), strings.Contains(normalized, "error"), strings.Contains(normalized, " err"):
		return "error"
	case strings.Contains(normalized, "warning"), strings.Contains(normalized, " warn"):
		return "warn"
	case strings.Contains(normalized, "debug"):
		return "debug"
	default:
		return "info"
	}
}
