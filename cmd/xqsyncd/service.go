package main

import (
	"context"
	"errors"
	"io"
	"os"
	"regexp"
)

// logOutput is where serve writes its log. A Windows service has no console, so the service entry
// point points it at the event log before serve starts.
var logOutput io.Writer = os.Stderr

// serviceName is what a service may be called: it becomes a registry key, the virtual account
// NT SERVICE\<name> and, by default, the data directory's name, so nothing a path could misread.
var serviceName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$`)

func validServiceName(name string) error {
	if !serviceName.MatchString(name) {
		return errors.New("xqsyncd service: a name of letters, digits, '.', '_' or '-', at most 80 characters")
	}
	return nil
}

// cmdService installs or removes the server as an operating-system service.
func cmdService(_ context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: xqsyncd service install|uninstall [--name NAME] ...")
	}
	switch args[0] {
	case "install":
		return installService(args[1:], out)
	case "uninstall":
		return uninstallService(args[1:], out)
	}
	return errors.New("usage: xqsyncd service install|uninstall [--name NAME] ...")
}
