package plugin_test

import (
	"testing"

	infragithub "ssh-client/internal/infra/github"
)

func TestParseChecksumsFile(t *testing.T) {
	content := "a1b2c3 demo-windows-amd64.exe\n# comment\ne5f6g7 demo-linux-amd64\n"
	got := infragithub.ParseChecksumsFile(content)
	if got["demo-windows-amd64.exe"] != "a1b2c3" {
		t.Fatalf("unexpected checksum map: %#v", got)
	}
	if got["demo-linux-amd64"] != "e5f6g7" {
		t.Fatalf("unexpected checksum map: %#v", got)
	}
}

func TestTotalDownloadCount(t *testing.T) {
	total := infragithub.TotalDownloadCount([]infragithub.Asset{
		{DownloadCount: 3},
		{DownloadCount: 7},
	})
	if total != 10 {
		t.Fatalf("got %d", total)
	}
}
