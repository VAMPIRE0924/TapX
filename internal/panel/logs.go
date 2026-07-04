package panel

import (
	"sync"
	"time"
)

const defaultLogLimit = 500

type LogEvent struct {
	Seq     uint64 `json:"seq"`
	Time    string `json:"time"`
	Level   string `json:"level"`
	Action  string `json:"action"`
	Message string `json:"message"`
}

type LogRecorder struct {
	mu     sync.Mutex
	limit  int
	next   uint64
	events []LogEvent
}

func NewLogRecorder(limit int) *LogRecorder {
	if limit <= 0 {
		limit = defaultLogLimit
	}
	return &LogRecorder{limit: limit}
}

func (r *LogRecorder) Add(level, action, message string) LogEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.next++
	event := LogEvent{
		Seq:     r.next,
		Time:    time.Now().UTC().Format(time.RFC3339Nano),
		Level:   level,
		Action:  action,
		Message: message,
	}
	r.events = append(r.events, event)
	if len(r.events) > r.limit {
		copy(r.events, r.events[len(r.events)-r.limit:])
		r.events = r.events[:r.limit]
	}
	return event
}

func (r *LogRecorder) List() []LogEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]LogEvent, len(r.events))
	copy(out, r.events)
	return out
}

func (r *LogRecorder) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = nil
}
