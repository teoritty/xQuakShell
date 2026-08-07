package wails

import (
	"context"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/usecase"
)

// recordingKnownHosts captures the context a handler passed down, which is the
// only way to tell an inherited app context from a fresh Background one.
type recordingKnownHosts struct{ got context.Context }

func (r *recordingKnownHosts) Check(string, domain.PublicKey) error { return nil }
func (r *recordingKnownHosts) List() ([]domain.KnownHostEntry, error) {
	return nil, nil
}
func (r *recordingKnownHosts) Add(ctx context.Context, _ string, _ domain.PublicKey) error {
	r.got = ctx
	return nil
}
func (r *recordingKnownHosts) Remove(ctx context.Context, _ string) error {
	r.got = ctx
	return nil
}
func (r *recordingKnownHosts) Replace(ctx context.Context, _ string, _ domain.PublicKey) error {
	r.got = ctx
	return nil
}

func TestReqCtxFallsBackWhenWailsHasNotSetTheContext(t *testing.T) {
	if got := (&AppAPI{}).reqCtx(); got == nil {
		t.Error("reqCtx returned nil before SetContext; every handler would panic on startup")
	}

	var nilAPI *AppAPI
	if got := nilAPI.reqCtx(); got == nil {
		t.Error("reqCtx on a nil receiver returned nil")
	}
}

func TestReqCtxReturnsTheAppContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	api := &AppAPI{ctx: ctx}
	if got := api.reqCtx(); got != ctx {
		t.Error("reqCtx did not return the context Wails handed to SetContext")
	}
}

// The point of the helper: work the UI starts must stop when the app does.
// A handler holding context.Background() keeps running against a backend that
// is already tearing down.
func TestHandlersRunUnderTheAppContext(t *testing.T) {
	appCtx, cancelApp := context.WithCancel(context.Background())
	repo := &recordingKnownHosts{}
	api := &AppAPI{
		ctx:      appCtx,
		hostKeys: usecase.NewHostKeyService(repo, nil),
	}

	if err := api.RemoveKnownHost("bastion.example"); err != nil {
		t.Fatalf("RemoveKnownHost: %v", err)
	}
	if repo.got == nil {
		t.Fatal("the handler passed no context down at all")
	}
	if repo.got.Err() != nil {
		t.Fatalf("the context reached the repository already done: %v", repo.got.Err())
	}

	cancelApp()

	if repo.got.Err() == nil {
		t.Error("cancelling the app context did not reach the context the handler passed down; the handler is detached from the app lifecycle")
	}
}
