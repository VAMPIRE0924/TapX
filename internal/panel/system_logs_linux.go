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
		eventTime, message := parseSystemLogLine(line)
		events = append(events, LogEvent{
			Seq:     uint64(len(events) + 1),
			Time:    eventTime,
			Level:   inferSystemLogLevel(line),
			Action:  "syslog",
			Message: message,
		})
	}
	if len(events) > limit {
		events = events[len(events)-limit:]
	}
	return events
}

func parseSystemLogLine(line string) (string, string) {
	fields := strings.Fields(line)
	if len(fields) >= 3 {
		if stamp, err := time.Parse("2006-01-02T15:04:05-0700", fields[0]); err == nil {
			message := strings.TrimSpace(strings.TrimPrefix(line, fields[0]))
			message = strings.TrimSpace(strings.TrimPrefix(message, fields[1]))
			return stamp.Format(time.RFC3339), message
		}
	}
	if len(fields) >= 7 {
		rawStamp := strings.Join(fields[:5], " ")
		if stamp, err := time.ParseInLocation("Mon Jan 2 15:04:05 2006", rawStamp, time.Local); err == nil {
			prefix := strings.Join(fields[:6], " ")
			message := strings.TrimSpace(strings.TrimPrefix(line, prefix))
			return stamp.Format(time.RFC3339), message
		}
	}
	return "", line
}

func inferSystemLogLevel(line string) string {
	normalized := strings.ToLower(line)
	switch {
	case strings.Contains(normalized, "panic"), strings.Contains(normalized, "fatal"),
		strings.Contains(normalized, "error"), strings.Contains(normalized, " err"),
		strings.Contains(normalized, ".err "), strings.Contains(normalized, "failed"):
		return "error"
	case strings.Contains(normalized, "warning"), strings.Contains(normalized, " warn"),
		strings.Contains(normalized, ".warning "):
		return "warn"
	case strings.Contains(normalized, "notice"), strings.Contains(normalized, ".notice "):
		return "notice"
	case strings.Contains(normalized, "debug"):
		return "debug"
	default:
		return "info"
	}
}
