package usecase

import (
	"sync"
	"time"

	"ssh-client/internal/domain"
)

// IdleLockoutManager implements domain.LockoutManager with idle timeout and minimize detection.
type IdleLockoutManager struct {
	mu           sync.Mutex
	settings     domain.LockoutSettings
	lastActivity time.Time
	minimized    bool
	handler      domain.LockoutEventHandler
	stopCh       chan struct{}
	running      bool
}

// NewIdleLockoutManager creates a lockout manager with the given settings.
func NewIdleLockoutManager(settings domain.LockoutSettings) *IdleLockoutManager {
	return &IdleLockoutManager{
		settings:     settings,
		lastActivity: time.Now(),
	}
}

// Start begins monitoring idle time. handler is called when lockout triggers.
func (m *IdleLockoutManager) Start(handler domain.LockoutEventHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return
	}

	m.handler = handler
	m.lastActivity = time.Now()
	m.stopCh = make(chan struct{})
	m.running = true

	go m.monitorLoop()
}

// Stop ceases monitoring.
func (m *IdleLockoutManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return
	}

	close(m.stopCh)
	m.running = false
}

// ReportActivity resets the idle timer.
func (m *IdleLockoutManager) ReportActivity() {
	m.mu.Lock()
	m.lastActivity = time.Now()
	m.mu.Unlock()
}

// ReportMinimized signals that the application was minimized.
func (m *IdleLockoutManager) ReportMinimized() {
	m.mu.Lock()
	wasMinimized := m.minimized
	m.minimized = true
	settings := m.settings
	handler := m.handler
	m.mu.Unlock()

	if !wasMinimized && settings.Enabled && settings.LockOnMinimize && handler != nil {
		handler()
	}
}

// ReportRestored signals that the application was restored from minimized.
func (m *IdleLockoutManager) ReportRestored() {
	m.mu.Lock()
	m.minimized = false
	m.lastActivity = time.Now()
	m.mu.Unlock()
}

// UpdateSettings applies new lockout configuration.
func (m *IdleLockoutManager) UpdateSettings(settings domain.LockoutSettings) {
	m.mu.Lock()
	m.settings = settings
	m.mu.Unlock()
}

// GetSettings returns current lockout configuration.
func (m *IdleLockoutManager) GetSettings() domain.LockoutSettings {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.settings
}

func (m *IdleLockoutManager) monitorLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkIdle()
		}
	}
}

func (m *IdleLockoutManager) checkIdle() {
	m.mu.Lock()
	settings := m.settings
	last := m.lastActivity
	handler := m.handler
	m.mu.Unlock()

	if !settings.Enabled || settings.IdleTimeout <= 0 {
		return
	}

	if time.Since(last) >= settings.IdleTimeout && handler != nil {
		handler()
		m.mu.Lock()
		m.lastActivity = time.Now()
		m.mu.Unlock()
	}
}
