package panel

import (
	"context"
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
	store  *Store
}

func NewLogRecorder(limit int) *LogRecorder {
	if limit <= 0 {
		limit = defaultLogLimit
	}
	return &LogRecorder{limit: limit}
}

func NewPersistentLogRecorder(store *Store, limit int) *LogRecorder {
	recorder := NewLogRecorder(limit)
	recorder.store = store
	if store == nil {
		return recorder
	}
	events, err := store.LoadLogs(context.Background(), recorder.limit)
	if err == nil {
		recorder.replace(events)
	}
	return recorder
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
	if r.store != nil {
		_ = r.store.AppendLog(context.Background(), event, r.limit)
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
	if r.store != nil {
		_ = r.store.ClearLogs(context.Background())
	}
}

func (r *LogRecorder) Replace(events []LogEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.replace(events)
}

func (r *LogRecorder) replace(events []LogEvent) {
	if len(events) > r.limit {
		events = events[len(events)-r.limit:]
	}
	r.events = append(r.events[:0], events...)
	r.next = 0
	for _, event := range r.events {
		if event.Seq > r.next {
			r.next = event.Seq
		}
	}
}
