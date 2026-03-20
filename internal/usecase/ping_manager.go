package usecase

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"ssh-client/internal/domain"
)

// PingResult holds the outcome of a single host TCP ping.
type PingResult struct {
	ConnectionID string `json:"connectionId"`
	Reachable    bool   `json:"reachable"`
	LatencyMs    int64  `json:"latencyMs"`
}

// PingEventHandler is called when ping results update.
type PingEventHandler func(results []PingResult)

// PingManager periodically checks TCP reachability for connections.
type PingManager struct {
	mu       sync.RWMutex
	settings domain.PingSettings
	results  map[string]PingResult
	handler  PingEventHandler
	cancel   context.CancelFunc
	connRepo domain.ConnectionRepository
}

// NewPingManager creates a new PingManager.
func NewPingManager(connRepo domain.ConnectionRepository, settings domain.PingSettings) *PingManager {
	return &PingManager{
		settings: settings,
		results:  make(map[string]PingResult),
		connRepo: connRepo,
	}
}

// Start begins periodic pinging.
func (pm *PingManager) Start(handler PingEventHandler) {
	pm.mu.Lock()
	pm.handler = handler
	if pm.cancel != nil {
		pm.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	pm.cancel = cancel
	pm.mu.Unlock()

	go pm.run(ctx)
}

// Stop halts periodic pinging.
func (pm *PingManager) Stop() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if pm.cancel != nil {
		pm.cancel()
		pm.cancel = nil
	}
}

// PingSingle pings a single connection immediately.
func (pm *PingManager) PingSingle(connID, host string, port int) {
	result := tcpPing(connID, host, port)
	pm.mu.Lock()
	pm.results[connID] = result
	handler := pm.handler
	pm.mu.Unlock()
	if handler != nil {
		handler(pm.GetResults())
	}
}

// GetResults returns a snapshot of current ping results.
func (pm *PingManager) GetResults() []PingResult {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	out := make([]PingResult, 0, len(pm.results))
	for _, r := range pm.results {
		out = append(out, r)
	}
	return out
}

// UpdateSettings reconfigures ping settings and restarts if needed.
func (pm *PingManager) UpdateSettings(s domain.PingSettings) {
	pm.mu.Lock()
	pm.settings = s
	pm.mu.Unlock()
}

func (pm *PingManager) run(ctx context.Context) {
	pm.pingAll(ctx)

	pm.mu.RLock()
	mode := pm.settings.Mode
	intervalSec := pm.settings.EffectiveIntervalSeconds()
	pm.mu.RUnlock()

	if mode != "" && mode != domain.PingModeInterval {
		// on_change mode: no ticker, just wait for context (PingSingle called from SaveConnection)
		<-ctx.Done()
		return
	}

	if intervalSec < 1 {
		intervalSec = 5
	}
	ticker := time.NewTicker(time.Duration(intervalSec) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pm.mu.RLock()
			enabled := pm.settings.Enabled
			m := pm.settings.Mode
			pm.mu.RUnlock()
			if enabled && (m == "" || m == domain.PingModeInterval) {
				pm.pingAll(ctx)
			}
		}
	}
}

func (pm *PingManager) pingAll(ctx context.Context) {
	conns, err := pm.connRepo.GetAllConnections(ctx)
	if err != nil {
		return
	}

	var wg sync.WaitGroup
	resultsCh := make(chan PingResult, len(conns))

	for _, c := range conns {
		host := c.EffectiveHost()
		port := c.EffectivePort()
		if host == "" || port <= 0 {
			continue
		}
		wg.Add(1)
		go func(id, h string, p int) {
			defer wg.Done()
			resultsCh <- tcpPing(id, h, p)
		}(c.ID, host, port)
	}

	wg.Wait()
	close(resultsCh)

	pm.mu.Lock()
	for r := range resultsCh {
		pm.results[r.ConnectionID] = r
	}
	handler := pm.handler
	pm.mu.Unlock()

	if handler != nil {
		handler(pm.GetResults())
	}
}

func tcpPing(connID, host string, port int) PingResult {
	addr := fmt.Sprintf("%s:%d", host, port)
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return PingResult{ConnectionID: connID, Reachable: false, LatencyMs: latency}
	}
	conn.Close()
	return PingResult{ConnectionID: connID, Reachable: true, LatencyMs: latency}
}
