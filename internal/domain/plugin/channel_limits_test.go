package plugin_test

import (
	"testing"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
)

func TestInitialCreditPerPurpose(t *testing.T) {
	cases := map[string]int{
		domainplugin.PurposeExec:        4,
		domainplugin.PurposeTCPRelay:    4,
		domainplugin.PurposeEmbedStream: 8,
	}
	for purpose, want := range cases {
		if got := domainplugin.InitialCredit(purpose); got != want {
			t.Fatalf("InitialCredit(%q) = %d, want %d", purpose, got, want)
		}
	}
}

func TestInitialCreditUnknownPurposeReturnsZero(t *testing.T) {
	if got := domainplugin.InitialCredit("bogus"); got != 0 {
		t.Fatalf("InitialCredit(bogus) = %d, want 0", got)
	}
}

func TestChannelLimitConstants(t *testing.T) {
	if domainplugin.MaxBinaryFrameBytes != 1<<20 {
		t.Fatalf("MaxBinaryFrameBytes = %d, want %d", domainplugin.MaxBinaryFrameBytes, 1<<20)
	}
	if domainplugin.FrameHeaderLen != 9 {
		t.Fatalf("FrameHeaderLen = %d, want 9", domainplugin.FrameHeaderLen)
	}
	if domainplugin.DefaultMaxConcurrentChannels != 4 {
		t.Fatalf("DefaultMaxConcurrentChannels = %d, want 4", domainplugin.DefaultMaxConcurrentChannels)
	}
	if domainplugin.DefaultChannelThroughputKbps != 32*1024 {
		t.Fatalf("DefaultChannelThroughputKbps = %d, want %d", domainplugin.DefaultChannelThroughputKbps, 32*1024)
	}
	if domainplugin.ChannelOpenTimeout != 10*time.Second {
		t.Fatalf("ChannelOpenTimeout = %v, want 10s", domainplugin.ChannelOpenTimeout)
	}
}

func TestFrameKindConstants(t *testing.T) {
	if domainplugin.FrameKindJSONRPC != 0x01 {
		t.Fatalf("FrameKindJSONRPC = %#x, want 0x01", domainplugin.FrameKindJSONRPC)
	}
	if domainplugin.FrameKindBinary != 0x02 {
		t.Fatalf("FrameKindBinary = %#x, want 0x02", domainplugin.FrameKindBinary)
	}
	if domainplugin.FrameKindCredit != 0x03 {
		t.Fatalf("FrameKindCredit = %#x, want 0x03", domainplugin.FrameKindCredit)
	}
}
