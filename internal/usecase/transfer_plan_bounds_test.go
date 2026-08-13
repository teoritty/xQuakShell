package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"xquakshell/internal/domain"
)

// endlessRemoteFS answers every List with one subdirectory, so the tree it describes has no bottom.
// A hostile or compromised server costs itself nothing to behave this way, and a FUSE mount or a
// custom SFTP implementation can do it without even meaning to.
type endlessRemoteFS struct {
	*fakeRemoteFS
	listCalls int
}

func (e *endlessRemoteFS) List(ctx context.Context, dir string) ([]domain.RemoteNode, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	e.listCalls++
	if dir == "/" {
		return []domain.RemoteNode{{Name: "deep", Path: "/deep", IsDir: true}}, nil
	}
	child := strings.TrimSuffix(dir, "/") + "/deep"
	return []domain.RemoteNode{{Name: "deep", Path: child, IsDir: true}}, nil
}

// The walk used to recurse on this until the stack gave out. Cancellation was the only brake, and
// it needs a user who has noticed something is wrong.
func TestWalkRemoteSourceStopsAtTheDepthLimit(t *testing.T) {
	fs := &endlessRemoteFS{fakeRemoteFS: &fakeRemoteFS{}}

	_, err := walkRemoteSource(context.Background(), fs, "/deep", nil)

	if !errors.Is(err, domain.ErrRemoteWalkTooDeep) {
		t.Fatalf("walkRemoteSource on an endless tree = %v, want ErrRemoteWalkTooDeep", err)
	}
	if e := fs.listCalls; e > domain.MaxRemoteWalkDepth+2 {
		t.Errorf("the walk made %d List calls before stopping, past a depth limit of %d",
			e, domain.MaxRemoteWalkDepth)
	}
}

// Reported, not truncated. A plan that quietly listed half a directory would copy half of it and
// report success, which is worse than refusing: the user believes the transfer completed.
func TestWalkRemoteSourceReportsRatherThanTruncating(t *testing.T) {
	fs := &endlessRemoteFS{fakeRemoteFS: &fakeRemoteFS{}}

	entries, err := walkRemoteSource(context.Background(), fs, "/deep", nil)

	if err == nil {
		t.Fatal("an endless tree produced no error")
	}
	if entries != nil {
		t.Errorf("walkRemoteSource returned %d entries alongside its error; a refused walk must "+
			"not hand back a partial tree that looks whole", len(entries))
	}
}

// wideRemoteFS serves one enormous flat directory: broad rather than deep, which the depth limit
// does not cover.
type wideRemoteFS struct {
	*fakeRemoteFS
	perDir int
}

func (w *wideRemoteFS) List(ctx context.Context, dir string) ([]domain.RemoteNode, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if dir == "/" {
		// walkRemoteSource finds its root by listing the parent, so the root has to be there.
		return []domain.RemoteNode{{Name: "wide", Path: "/wide", IsDir: true}}, nil
	}
	// Every directory holds perDir subdirectories, so the entry count explodes long before the
	// depth limit is reached.
	out := make([]domain.RemoteNode, 0, w.perDir)
	for i := range w.perDir {
		name := string(rune('a'+i%26)) + string(rune('0'+i/26%10)) + string(rune('0'+i/260%10))
		out = append(out, domain.RemoteNode{
			Name:  name,
			Path:  strings.TrimSuffix(dir, "/") + "/" + name,
			IsDir: true,
		})
	}
	return out, nil
}

func TestWalkRemoteSourceStopsAtTheEntryLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("enumerates up to the entry limit")
	}
	fs := &wideRemoteFS{fakeRemoteFS: &fakeRemoteFS{}, perDir: 64}

	_, err := walkRemoteSource(context.Background(), fs, "/wide", nil)

	if !errors.Is(err, domain.ErrRemoteWalkTooLarge) && !errors.Is(err, domain.ErrRemoteWalkTooDeep) {
		t.Fatalf("walkRemoteSource on an exploding tree = %v, want one of the walk bounds", err)
	}
}

// The bounds must not fire on anything a person actually transfers, or the fix removes the feature.
func TestWalkRemoteSourceWalksAnOrdinaryTree(t *testing.T) {
	fs := &walkRemoteFS{fakeRemoteFS: &fakeRemoteFS{}, width: 3, depth: 4}

	entries, err := walkRemoteSource(context.Background(), fs, "/src", nil)

	if err != nil {
		t.Fatalf("walkRemoteSource on an ordinary tree = %v, want nil", err)
	}
	if len(entries) == 0 {
		t.Error("an ordinary tree enumerated nothing")
	}
}
