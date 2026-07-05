package ratelimit

import (
	"testing"
)

func TestLimiterAllowNRespectsBurst(t *testing.T) {
	l := New(10, 1)

	if !l.AllowN(1) {
		t.Fatal("expected first AllowN(1) to succeed")
	}
	if l.AllowN(1) {
		t.Fatal("expected second AllowN(1) to fail with burst=1")
	}
}

func TestLimiterAllowNLargeBurst(t *testing.T) {
	const frameSize = 64 * 1024
	l := New(32*1024*1024, frameSize)

	if !l.AllowN(frameSize) {
		t.Fatalf("expected AllowN(%d) to succeed with burst=%d", frameSize, frameSize)
	}
}

func TestFactoryImplementsDomainPort(t *testing.T) {
	var factory Factory
	l := factory.New(10, 1)
	if l == nil {
		t.Fatal("expected non-nil limiter from Factory.New")
	}
	if !l.AllowN(1) {
		t.Fatal("expected AllowN(1) to succeed")
	}
}
