package usecase

import (
	"context"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"time"

	"ssh-client/internal/domain"
	"ssh-client/internal/pkg/safego"
)

// PingResult holds the outcome of a single host TCP ping.
type PingResult struct {
	ConnectionID string `json:"connectionId"`
	Reachable    bool   `json:"reachable"`
	LatencyMs    int64  `json:"latencyMs"`
}

// PingEventHandler is called when ping results update.
type PingEventHandler func(results []PingResult)

type pingJob struct {
	connID string
	host   string
	port   int
}

// PingManager periodically checks TCP reachability for connections.
type PingManager struct {
	mu             sync.RWMutex
	settings       domain.PingSettings
	results        map[string]PingResult
	handler        PingEventHandler
	cancel         context.CancelFunc
	connRepo       domain.ConnectionRepository
	protocolLookup domain.ConnectionProtocolLookup
	limiter        domain.ConcurrencyLimiter
	pinger         domain.Pinger
	cycleMu        sync.Mutex
	cycleRunning   bool
}

// NewPingManager creates a new PingManager.
func NewPingManager(connRepo domain.ConnectionRepository, settings domain.PingSettings, limiter domain.ConcurrencyLimiter, pinger domain.Pinger) *PingManager {
	if limiter == nil {
		panic("usecase: PingManager requires ConcurrencyLimiter")
	}
	if pinger == nil {
		panic("usecase: PingManager requires Pinger")
	}
	return &PingManager{
		settings: settings,
		results:  make(map[string]PingResult),
		connRepo: connRepo,
		limiter:  limiter,
		pinger:   pinger,
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

	safego.GoNamed("ping.run", func() { pm.run(ctx) })
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

// SetProtocolLookup configures default port resolution for plugin protocols.
func (pm *PingManager) SetProtocolLookup(lookup domain.ConnectionProtocolLookup) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.protocolLookup = lookup
}

func (pm *PingManager) effectivePort(conn domain.Connection) int {
	pm.mu.RLock()
	lookup := pm.protocolLookup
	pm.mu.RUnlock()
	return conn.EffectivePort(lookup)
}

func (pm *PingManager) currentSettings() domain.PingSettings {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.settings
}

// PingByConnectionID loads a connection and pings it immediately.
func (pm *PingManager) PingByConnectionID(ctx context.Context, connID string) {
	if pm == nil || pm.connRepo == nil {
		return
	}
	conn, err := pm.connRepo.GetByID(ctx, connID)
	if err != nil {
		return
	}
	host := conn.EffectiveHost()
	port := pm.effectivePort(*conn)
	if host == "" || port <= 0 {
		return
	}
	safego.GoNamed("ping.single", func() { pm.PingSingle(ctx, connID, host, port) })
}

// PingSingle pings a single connection immediately.
func (pm *PingManager) PingSingle(ctx context.Context, connID, host string, port int) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := pm.limiter.Acquire(ctx); err != nil {
		return
	}
	defer pm.limiter.Release()

	result := pm.tcpPing(ctx, connID, host, port)
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
	pm.limiter.SetLimit(s.EffectiveMaxConcurrent())
}

func (pm *PingManager) run(ctx context.Context) {
	pm.pingAll(ctx)

	settings := pm.currentSettings()
	mode := settings.Mode
	intervalSec := settings.EffectiveIntervalSeconds()

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
			settings = pm.currentSettings()
			if settings.Enabled && (settings.Mode == "" || settings.Mode == domain.PingModeInterval) {
				pm.pingAll(ctx)
			}
		}
	}
}

func (pm *PingManager) tryBeginCycle() bool {
	pm.cycleMu.Lock()
	defer pm.cycleMu.Unlock()
	if pm.cycleRunning {
		return false
	}
	pm.cycleRunning = true
	return true
}

func (pm *PingManager) endCycle() {
	pm.cycleMu.Lock()
	pm.cycleRunning = false
	pm.cycleMu.Unlock()
}

func (pm *PingManager) pingAll(ctx context.Context) {
	if !pm.tryBeginCycle() {
		slog.Debug("ping: cycle skipped, previous still running")
		return
	}
	defer pm.endCycle()

	conns, err := pm.connRepo.GetAllConnections(ctx)
	if err != nil {
		slog.Warn("ping: failed to load connections", "err", err)
		return
	}

	jobs := make([]pingJob, 0, len(conns))
	for _, c := range conns {
		host := c.EffectiveHost()
		port := pm.effectivePort(c)
		if host == "" || port <= 0 {
			continue
		}
		jobs = append(jobs, pingJob{connID: c.ID, host: host, port: port})
	}
	if len(jobs) == 0 {
		return
	}

	settings := pm.currentSettings()
	workers := settings.EffectiveMaxConcurrent()
	if workers > len(jobs) {
		workers = len(jobs)
	}

	jobsCh := make(chan pingJob, len(jobs))
	for _, job := range jobs {
		jobsCh <- job
	}
	close(jobsCh)

	resultsCh := make(chan PingResult, len(jobs))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		safego.GoNamed("ping.worker", func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-jobsCh:
					if !ok {
						return
					}
					resultsCh <- pm.tcpPing(ctx, job.connID, job.host, job.port)
				}
			}
		})
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

func (pm *PingManager) tcpPing(ctx context.Context, connID, host string, port int) PingResult {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	start := time.Now()
	err := pm.pinger.Ping(ctx, "tcp", addr)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return PingResult{ConnectionID: connID, Reachable: false, LatencyMs: latency}
	}
	return PingResult{ConnectionID: connID, Reachable: true, LatencyMs: latency}
}
