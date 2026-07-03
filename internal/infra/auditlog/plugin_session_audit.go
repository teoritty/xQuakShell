package auditlog

import (
	"sync"
	"time"
)

// PluginSessionAuditEntry records a plugin session lifecycle event without secret values.
type PluginSessionAuditEntry struct {
	Timestamp    time.Time `json:"timestamp"`
	PluginID     string    `json:"pluginId"`
	Action       string    `json:"action"`
	ConnectionID string    `json:"connectionId,omitempty"`
	Protocol     string    `json:"protocol,omitempty"`
	FieldCount   int       `json:"fieldCount,omitempty"`
	Success      bool      `json:"success"`
	Error        string    `json:"error,omitempty"`
}

// PluginSessionAuditLog is a fixed-size ring buffer of plugin session audit entries.
type PluginSessionAuditLog struct {
	mu       sync.Mutex
	buffer   []PluginSessionAuditEntry
	head     int
	tail     int
	size     int
	capacity int
}

// NewPluginSessionAuditLog creates a ring buffer with the given capacity.
func NewPluginSessionAuditLog(capacity int) *PluginSessionAuditLog {
	if capacity < 1 {
		capacity = 256
	}
	return &PluginSessionAuditLog{
		buffer:   make([]PluginSessionAuditEntry, capacity),
		capacity: capacity,
	}
}

// Record appends an entry, evicting the oldest when full.
func (l *PluginSessionAuditLog) Record(entry PluginSessionAuditEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.buffer[l.tail] = entry
	l.tail = (l.tail + 1) % l.capacity

	if l.size < l.capacity {
		l.size++
	} else {
		l.head = (l.head + 1) % l.capacity
	}
}

// GetAll returns all entries in chronological order.
func (l *PluginSessionAuditLog) GetAll() []PluginSessionAuditEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	result := make([]PluginSessionAuditEntry, l.size)
	for i := 0; i < l.size; i++ {
		idx := (l.head + i) % l.capacity
		result[i] = l.buffer[idx]
	}
	return result
}
