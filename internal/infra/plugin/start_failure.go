package plugin

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// startFailureExitGrace is how long a failed start waits to learn how the child died.
//
// It is short because the answer is almost always already there: a handshake that ends in EOF ends
// that way because the process on the other end is gone, and the reaper is waiting on it in
// parallel. The wait exists only to lose a race with the kernel, not to give a live process time to
// change its mind.
const startFailureExitGrace = 2 * time.Second

// explainStartFailure adds how the plugin process died to the error that says the start failed.
//
// "plugin initialize: EOF" is what a caller sees when the child closed its stdout, and on its own it
// names a symptom with no cause. It became a real problem once a sandbox could refuse to apply: a
// plugin that will not start is the failure mode of that whole design, and the first thing anyone
// looks at is an error that says nothing about whether the process exited, was killed, or never ran.
//
// A process that is still alive is left alone and the original error is returned unchanged — the
// handshake can fail for reasons that have nothing to do with the process's health.
func (h *ProcessHost) explainStartFailure(mp *managedProcess, err error) error {
	if err == nil || mp == nil || mp.reaper == nil {
		return err
	}
	select {
	case <-mp.reaper.Done():
	case <-time.After(startFailureExitGrace):
		return err
	}
	exit := mp.reaper.ExitErr()
	how := "exited cleanly before the handshake finished"
	if exit != nil {
		how = exit.Error()
	}
	return fmt.Errorf("%w (the plugin process %s)%s", err, how, stderrTailSuffix(mp.stderr))
}

// stderrTailSuffix renders whatever the process said on its way out, or nothing at all when it said
// nothing. A trailing "; last stderr: []" on every failure would be noise on the many that have no
// output to show.
func stderrTailSuffix(stderr io.WriteCloser) string {
	writer, ok := stderr.(*redactingStderrWriter)
	if !ok {
		return ""
	}
	tail := writer.Tail()
	if len(tail) == 0 {
		return ""
	}
	return "; last stderr: " + strings.Join(tail, " | ")
}
