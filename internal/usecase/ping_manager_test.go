package usecase

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ssh-client/internal/domain"
)

type stubConnRepo struct {
	conns []domain.Connection
}

func (s *stubConnRepo) GetAllFolders(context.Context) ([]domain.ConnectionFolder, error) {
	return nil, nil
}
func (s *stubConnRepo) SaveFolder(context.Context, *domain.ConnectionFolder) error { return nil }
func (s *stubConnRepo) DeleteFolder(context.Context, string) error               { return nil }
func (s *stubConnRepo) GetAllConnections(context.Context) ([]domain.Connection, error) {
	return s.conns, nil
}
func (s *stubConnRepo) GetByFolder(context.Context, string) ([]domain.Connection, error) {
	return nil, nil
}
func (s *stubConnRepo) GetByID(_ context.Context, id string) (*domain.Connection, error) {
	for i := range s.conns {
		if s.conns[i].ID == id {
			c := s.conns[i]
			return &c, nil
		}
	}
	return nil, errors.New("not found")
}
func (s *stubConnRepo) Save(context.Context, *domain.Connection) error { return nil }
func (s *stubConnRepo) Delete(context.Context, string) error           { return nil }
func (s *stubConnRepo) MoveToFolder(context.Context, []string, string) error {
	return nil
}
func (s *stubConnRepo) MoveFolder(context.Context, string, string) error { return nil }
func (s *stubConnRepo) ReorderConnections(context.Context, []string, string) error {
	return nil
}
func (s *stubConnRepo) ReorderFolders(context.Context, []string, string) error { return nil }

type recordingPinger struct {
	mu     sync.Mutex
	active int
	peak   int
	delay  time.Duration
	pings  int32
}

func (p *recordingPinger) Ping(ctx context.Context, network, address string) error {
	atomic.AddInt32(&p.pings, 1)
	p.mu.Lock()
	p.active++
	if p.active > p.peak {
		p.peak = p.active
	}
	delay := p.delay
	p.mu.Unlock()

	select {
	case <-ctx.Done():
		p.mu.Lock()
		p.active--
		p.mu.Unlock()
		return ctx.Err()
	case <-time.After(delay):
	}

	p.mu.Lock()
	p.active--
	p.mu.Unlock()
	return nil
}

type stubConcurrencyLimiter struct {
	mu     sync.Mutex
	cond   *sync.Cond
	active int
	limit  int
}

func newStubConcurrencyLimiter(limit int) *stubConcurrencyLimiter {
	if limit < 1 {
		limit = 1
	}
	l := &stubConcurrencyLimiter{limit: limit}
	l.cond = sync.NewCond(&l.mu)
	return l
}

func (l *stubConcurrencyLimiter) Acquire(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			l.cond.Broadcast()
		case <-done:
		}
	}()
	l.mu.Lock()
	for l.active >= l.limit {
		l.cond.Wait()
		if ctx.Err() != nil {
			l.mu.Unlock()
			return ctx.Err()
		}
	}
	l.active++
	l.mu.Unlock()
	return nil
}

func (l *stubConcurrencyLimiter) Release() {
	l.mu.Lock()
	l.active--
	l.cond.Signal()
	l.mu.Unlock()
}

func (l *stubConcurrencyLimiter) SetLimit(limit int) {
	if limit < 1 {
		limit = 1
	}
	l.mu.Lock()
	l.limit = limit
	l.cond.Broadcast()
	l.mu.Unlock()
}

func testConnections(n int) []domain.Connection {
	conns := make([]domain.Connection, n)
	for i := range conns {
		conns[i] = domain.Connection{
			ID:   fmt.Sprintf("conn-%d", i),
			Host: "host.example",
			Port: 22,
		}
	}
	return conns
}

func newTestPingManager(conns []domain.Connection, maxConcurrent int, pinger domain.Pinger) *PingManager {
	return NewPingManager(&stubConnRepo{conns: conns}, domain.PingSettings{
		Enabled:       true,
		Mode:          domain.PingModeInterval,
		MaxConcurrent: maxConcurrent,
	}, newStubConcurrencyLimiter(maxConcurrent), pinger)
}

func TestNewPingManagerPanicsOnNilPinger(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil Pinger")
		}
	}()
	NewPingManager(&stubConnRepo{}, domain.DefaultPingSettings(), newStubConcurrencyLimiter(1), nil)
}

func TestPingAllRespectsMaxConcurrent(t *testing.T) {
	pinger := &recordingPinger{delay: 50 * time.Millisecond}
	pm := newTestPingManager(testConnections(50), 4, pinger)

	pm.pingAll(context.Background())

	if pinger.peak > 4 {
		t.Fatalf("peak concurrent = %d, want <= 4", pinger.peak)
	}
	if got := len(pm.GetResults()); got != 50 {
		t.Fatalf("results = %d, want 50", got)
	}
}

func TestPingAllSingleFlight(t *testing.T) {
	pinger := &recordingPinger{delay: 200 * time.Millisecond}
	pm := newTestPingManager(testConnections(10), 2, pinger)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		pm.pingAll(context.Background())
	}()
	go func() {
		defer wg.Done()
		time.Sleep(20 * time.Millisecond)
		pm.pingAll(context.Background())
	}()
	wg.Wait()

	if got := atomic.LoadInt32(&pinger.pings); got != 10 {
		t.Fatalf("pings = %d, want 10 (second cycle skipped)", got)
	}
}

func TestPingAllContextCancel(t *testing.T) {
	pinger := &recordingPinger{delay: 500 * time.Millisecond}
	pm := newTestPingManager(testConnections(20), 2, pinger)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		pm.pingAll(ctx)
		close(done)
	}()

	time.Sleep(80 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pingAll did not return after context cancel")
	}
}

func TestPingSingleUsesLimiter(t *testing.T) {
	pinger := &recordingPinger{delay: 100 * time.Millisecond}
	pm := newTestPingManager(nil, 2, pinger)

	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			pm.PingSingle(ctx, fmt.Sprintf("conn-%d", n), "host.example", 22)
		}(i)
	}
	wg.Wait()

	if pinger.peak > 2 {
		t.Fatalf("peak concurrent = %d, want <= 2", pinger.peak)
	}
}
