//go:build linux

package panel

import "testing"

func TestSystemLogEventsLimitsAndClassifies(t *testing.T) {
	events := systemLogEvents(
		"2026-07-26T17:28:11+0800 host tapx-panel[10]: old info\n"+
			"Sun Jul 26 17:14:05 2026 daemon.warning tapx-panel[20]: TapX WARNING retry\n"+
			"Sun Jul 26 17:14:09 2026 daemon.err tapx-panel[20]: TapX ERROR stopped\n",
		2,
	)
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if events[0].Level != "warn" || events[1].Level != "error" {
		t.Fatalf("levels = %q, %q", events[0].Level, events[1].Level)
	}
	if events[0].Action != "syslog" || events[1].Action != "syslog" {
		t.Fatalf("actions = %q, %q", events[0].Action, events[1].Action)
	}
	if events[0].Time == "" || events[1].Time == "" {
		t.Fatalf("times = %q, %q, want parsed timestamps", events[0].Time, events[1].Time)
	}
	if events[0].Message != "tapx-panel[20]: TapX WARNING retry" {
		t.Fatalf("message = %q", events[0].Message)
	}
}

func TestParseSystemLogLineKeepsUnknownFormat(t *testing.T) {
	eventTime, message := parseSystemLogLine("custom syslog line")
	if eventTime != "" || message != "custom syslog line" {
		t.Fatalf("time=%q message=%q", eventTime, message)
	}
}
