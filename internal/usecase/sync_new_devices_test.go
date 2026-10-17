package usecase

import (
	"encoding/hex"
	"testing"

	"xquakshell/internal/domain/vaultsync"
)

func addEntry(device byte, name string) vaultsync.DeviceEntry {
	return vaultsync.DeviceEntry{Op: vaultsync.OpAddDevice, Cert: &vaultsync.DeviceCert{Device: vaultsync.ID{device}, Name: name}}
}

func names(n []NewDeviceNotice) []string {
	var out []string
	for _, d := range n {
		out = append(out, d.Name)
	}
	return out
}

func TestOnlyDevicesAfterThisOneAreNews(t *testing.T) {
	log := []vaultsync.DeviceEntry{addEntry(1, "laptop"), addEntry(2, "phone"), addEntry(3, "tablet")}
	if got := names(newDeviceNotices(log, vaultsync.ID{2}, nil)); len(got) != 1 || got[0] != "tablet" {
		t.Fatalf("notices = %v; the device before this one is not news here, the one after is", got)
	}
	if got := newDeviceNotices(log, vaultsync.ID{3}, nil); len(got) != 0 {
		t.Fatalf("notices = %v; a device never announces itself or what came before it", names(got))
	}
	if got := newDeviceNotices(log, vaultsync.ID{9}, nil); len(got) != 0 {
		t.Fatalf("notices = %v; with this device not in the log yet, nothing is news", names(got))
	}
}

func TestAcknowledgedAndRevokedDevicesAreNotAnnounced(t *testing.T) {
	log := []vaultsync.DeviceEntry{addEntry(1, "laptop"), addEntry(2, "phone"), addEntry(3, "tablet")}
	phone := vaultsync.ID{2}
	if got := names(newDeviceNotices(log, vaultsync.ID{1}, []string{hex.EncodeToString(phone[:])})); len(got) != 1 || got[0] != "tablet" {
		t.Fatalf("notices = %v; an acknowledged device is not announced again", got)
	}
	revoked := append(log, vaultsync.DeviceEntry{Op: vaultsync.OpRevokeDevice, Revoked: vaultsync.ID{3}})
	if got := names(newDeviceNotices(revoked, vaultsync.ID{1}, nil)); len(got) != 1 || got[0] != "phone" {
		t.Fatalf("notices = %v; a removed device needs no more attention", got)
	}
}
