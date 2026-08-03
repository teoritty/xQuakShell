package plugin_test

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestReaperWaitOnce(t *testing.T) {
	cmd := exec.Command("go", "version")
	if err := cmd.Start(); err != nil {
		t.Skip("cannot start helper process:", err)
	}

	exited := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		close(exited)
	}()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-exited:
			case <-time.After(5 * time.Second):
				t.Error("timeout waiting for exit")
			}
		}()
	}
	wg.Wait()
	<-exited
	if waitErr != nil && !isBenignExit(waitErr) {
		t.Fatalf("unexpected wait error: %v", waitErr)
	}
}

func TestReaperKillAndWait(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sleep helper differs on windows")
	}
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Skip("cannot start helper:", err)
	}

	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()

	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("process did not exit after kill")
	}
}

func isBenignExit(err error) bool {
	if err == nil {
		return true
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode() == 0
	}
	return false
}

func TestReaperConcurrentAfterExit(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()
	<-exited

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for i := 0; i < 50; i++ {
		select {
		case <-exited:
		case <-ctx.Done():
			t.Fatal("exited channel not closed")
		}
	}
}
