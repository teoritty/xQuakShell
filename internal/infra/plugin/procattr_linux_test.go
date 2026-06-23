//go:build linux

package plugin

import (
	"os/exec"
	"syscall"
	"testing"
)

func TestConfigurePluginCmdSetsPdeathsigAndProcessGroup(t *testing.T) {
	cmd := exec.Command("true")
	if err := configurePluginCmd(cmd); err != nil {
		t.Fatal(err)
	}
	attr, ok := cmd.SysProcAttr.(*syscall.SysProcAttr)
	if !ok || attr == nil {
		t.Fatal("expected SysProcAttr")
	}
	if !attr.Setpgid {
		t.Fatal("expected Setpgid")
	}
	if attr.Pdeathsig != syscall.SIGKILL {
		t.Fatalf("expected Pdeathsig SIGKILL, got %v", attr.Pdeathsig)
	}
}
