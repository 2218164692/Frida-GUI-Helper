package logstream

import (
	"sync"
	"time"

	"frida-gui-helper/internal/diagnostics"
)

type Level string

const (
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

type Entry struct {
	Time       string               `json:"time"`
	Level      Level                `json:"level"`
	Source     string               `json:"source"`
	Message    string               `json:"message"`
	Diagnostic *diagnostics.Finding `json:"diagnostic,omitempty"`
}

type Emitter func(Entry)

type Stream struct {
	mu      sync.Mutex
	max     int
	entries []Entry
	emit    Emitter
}

func New(max int, emit Emitter) *Stream {
	if max <= 0 {
		max = 500
	}
	return &Stream{max: max, emit: emit}
}

func (s *Stream) SetEmitter(emit Emitter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emit = emit
}

func (s *Stream) Add(level Level, source string, message string) Entry {
	return s.AddWithDiagnostic(level, source, message, nil)
}

func (s *Stream) AddWithDiagnostic(level Level, source string, message string, finding *diagnostics.Finding) Entry {
	if level == "" {
		level = LevelInfo
	}

	entry := Entry{
		Time:       time.Now().Format("15:04:05"),
		Level:      level,
		Source:     source,
		Message:    message,
		Diagnostic: finding,
	}

	var emit Emitter
	s.mu.Lock()
	s.entries = append(s.entries, entry)
	if len(s.entries) > s.max {
		s.entries = append([]Entry(nil), s.entries[len(s.entries)-s.max:]...)
	}
	emit = s.emit
	s.mu.Unlock()

	if emit != nil {
		emit(entry)
	}
	return entry
}

func (s *Stream) Entries() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()

	copied := make([]Entry, len(s.entries))
	copy(copied, s.entries)
	return copied
}

func (s *Stream) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = nil
}
