package usecase

import (
	"io"

	"xquakshell/internal/pkg/safego"
)

// splice copies bytes both directions between a and b until either side
// closes or errors, then closes both ends. Blocks until done — callers run
// it in its own goroutine via safego.Go.
func splice(a, b io.ReadWriteCloser) {
	done := make(chan struct{}, 2)
	safego.Go(func() { io.Copy(a, b); done <- struct{}{} })
	safego.Go(func() { io.Copy(b, a); done <- struct{}{} })
	<-done
	a.Close()
	b.Close()
}
