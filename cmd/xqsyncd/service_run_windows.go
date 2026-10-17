//go:build windows

package main

import (
	"context"
	"io"
	"strings"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"

	"xquakshell/internal/pkg/safego"
)

// runIfService runs the command line under the service control manager when the process was
// started by it, and reports whether it did.
func runIfService(args []string) (bool, error) {
	isService, err := svc.IsWindowsService()
	if err != nil || !isService {
		return false, err
	}
	name := serviceNameOf(args)
	if el, err := eventlog.Open(name); err == nil {
		defer func() { _ = el.Close() }()
		logOutput = eventLogWriter{el}
	} else {
		logOutput = io.Discard
	}
	return true, svc.Run(name, serviceHandler{args: args})
}

// serviceNameOf is the event source to log to. The installer names it after the service, and the
// data directory defaults to the same name; the SCM does not tell a process its own name.
func serviceNameOf(args []string) string {
	for i, a := range args {
		if (a == "--dir" || a == "-dir") && i+1 < len(args) {
			if base := lastPathElement(args[i+1]); validServiceName(base) == nil {
				return base
			}
		}
	}
	return "xqsyncd"
}

func lastPathElement(p string) string {
	p = strings.TrimRight(p, `\/`)
	if i := strings.LastIndexAny(p, `\/`); i >= 0 {
		return p[i+1:]
	}
	return p
}

// eventLogWriter sends each log line to the Windows event log, at error level for slog's ERROR.
type eventLogWriter struct{ el *eventlog.Log }

func (w eventLogWriter) Write(p []byte) (int, error) {
	line := strings.TrimSpace(string(p))
	var err error
	switch {
	case strings.Contains(line, "level=ERROR"):
		err = w.el.Error(1, line)
	case strings.Contains(line, "level=WARN"):
		err = w.el.Warning(1, line)
	default:
		err = w.el.Info(1, line)
	}
	return len(p), err
}

type serviceHandler struct{ args []string }

// Execute runs the server until the service control manager stops it. Stop and shutdown cancel the
// serve context, which drains in-flight requests within serve's ten-second shutdown.
func (h serviceHandler) Execute(_ []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	// The service control manager owns this process's lifetime: this is its root context.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	safego.GoNamed("xqsyncd.service", func() { done <- run(ctx, h.args, io.Discard) })
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case err := <-done:
			return false, exitCode(err)
		case c := <-req:
			switch c.Cmd {
			case svc.Interrogate:
				status <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				cancel()
				return false, exitCode(<-done)
			}
		}
	}
}

func exitCode(err error) uint32 {
	if err != nil {
		return 1
	}
	return 0
}
