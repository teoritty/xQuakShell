package unit_test

import (
	"sync/atomic"
	"testing"
	"time"

	"ssh-client/internal/domain"
	"ssh-client/internal/usecase"
)

func TestLockoutTriggersOnIdle(t *testing.T) {
	var counter atomic.Int32
	settings := domain.LockoutSettings{
		Enabled:        true,
		IdleTimeout:    6 * time.Second,
		LockOnMinimize: false,
	}

	mgr := usecase.NewIdleLockoutManager(settings)
	mgr.Start(func() { counter.Add(1) })
	defer mgr.Stop()

	time.Sleep(12 * time.Second)
	if counter.Load() == 0 {
		t.Error("expected lockout handler to be called after idle timeout")
	}
}

func TestLockoutResetByActivity(t *testing.T) {
	var counter atomic.Int32
	settings := domain.LockoutSettings{
		Enabled:        true,
		IdleTimeout:    8 * time.Second,
		LockOnMinimize: false,
	}

	mgr := usecase.NewIdleLockoutManager(settings)
	mgr.Start(func() { counter.Add(1) })
	defer mgr.Stop()

	time.Sleep(100 * time.Millisecond)
	mgr.ReportActivity()
	time.Sleep(100 * time.Millisecond)
	mgr.ReportActivity()
	time.Sleep(100 * time.Millisecond)
	mgr.ReportActivity()

	if counter.Load() != 0 {
		t.Errorf("expected no trigger while activity resets timer, got counter=%d", counter.Load())
	}

	time.Sleep(12 * time.Second)
	if counter.Load() == 0 {
		t.Error("expected lockout handler to be called after prolonged idle")
	}
}

func TestLockoutMinimize(t *testing.T) {
	var counter atomic.Int32
	settings := domain.LockoutSettings{
		Enabled:        true,
		IdleTimeout:    5 * time.Minute,
		LockOnMinimize: true,
	}

	mgr := usecase.NewIdleLockoutManager(settings)
	mgr.Start(func() { counter.Add(1) })
	defer mgr.Stop()

	mgr.ReportMinimized()

	if counter.Load() != 1 {
		t.Errorf("expected counter=1 on minimize, got %d", counter.Load())
	}
}

func TestLockoutDisabled(t *testing.T) {
	var counter atomic.Int32
	settings := domain.LockoutSettings{
		Enabled:        false,
		IdleTimeout:    6 * time.Second,
		LockOnMinimize: true,
	}

	mgr := usecase.NewIdleLockoutManager(settings)
	mgr.Start(func() { counter.Add(1) })
	defer mgr.Stop()

	time.Sleep(12 * time.Second)
	if counter.Load() != 0 {
		t.Errorf("expected no trigger when disabled, got counter=%d", counter.Load())
	}
}
