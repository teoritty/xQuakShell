package plugin

import (
	"context"
	"os/exec"
	"sync"
)

// processReaper owns the single (*exec.Cmd).Wait call for a plugin child process.
type processReaper struct {
	cmd    *exec.Cmd
	exited chan struct{}
	err    error
	once   sync.Once
}

func newProcessReaper(cmd *exec.Cmd) *processReaper {
	return &processReaper{
		cmd:    cmd,
		exited: make(chan struct{}),
	}
}

// Start launches the background goroutine that waits on the child process.
func (r *processReaper) Start() {
	r.once.Do(func() {
		go r.wait()
	})
}

func (r *processReaper) wait() {
	r.err = r.cmd.Wait()
	close(r.exited)
}

// Wait blocks until the child exits or ctx is cancelled.
func (r *processReaper) Wait(ctx context.Context) error {
	select {
	case <-r.exited:
		return r.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Kill terminates the child and waits for exit.
func (r *processReaper) Kill() error {
	if r.cmd.Process != nil {
		killPluginProcess(r.cmd.Process.Pid)
	}
	return r.Wait(context.Background())
}

// Done returns a channel closed when the child has exited.
func (r *processReaper) Done() <-chan struct{} {
	return r.exited
}

// ExitErr returns the error from Wait after the process has exited.
func (r *processReaper) ExitErr() error {
	<-r.exited
	return r.err
}
