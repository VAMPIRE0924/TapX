//go:build linux

package panel

import "testing"

func TestSystemLogEventsLimitsAndClassifies(t *testing.T) {
	events := systemLogEvents("old info\nTapX WARNING retry\nTapX ERROR stopped\n", 2)
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if events[0].Level != "warn" || events[1].Level != "error" {
		t.Fatalf("levels = %q, %q", events[0].Level, events[1].Level)
	}
	if events[0].Action != "syslog" || events[1].Action != "syslog" {
		t.Fatalf("actions = %q, %q", events[0].Action, events[1].Action)
	}
}
