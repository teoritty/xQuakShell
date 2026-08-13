package plugin_test

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

func boundsProxyDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "seed.txt"), []byte("seed"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return dir
}

// fs.list read the whole directory before anything was filtered, so the host held one entry per
// name in whatever directory the plugin pointed at — and a plugin can manufacture that directory
// itself inside its own write root. The plugin's process is memory-capped by its job object or
// rlimit; the host is not, which is what makes this the host's problem.
func TestListRefusesADirectoryPastTheEntryCap(t *testing.T) {
	dir := boundsProxyDir(t)
	for i := 0; i <= domainplugin.MaxListEntries; i++ {
		name := filepath.Join(dir, fmt.Sprintf("e%06d", i))
		if err := os.WriteFile(name, nil, 0o600); err != nil {
			t.Fatalf("populate: %v", err)
		}
	}
	fs := mustFSProxy(t, &domainplugin.FSCaps{Read: []string{"${pluginData}"}}, dir)

	_, err := fs.Handle("fs.list", mustJSON(map[string]any{"path": "."}))

	if !errors.Is(err, domainplugin.ErrDirectoryTooLarge) {
		t.Fatalf("err = %v, want ErrDirectoryTooLarge", err)
	}
}

// The cap must not turn into a silent truncation, and an ordinary directory must be served whole:
// a listing missing entries nobody can count is worse than an error.
func TestListServesAnOrdinaryDirectoryWhole(t *testing.T) {
	dir := boundsProxyDir(t)
	const count = 50
	for i := range count {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%03d", i)), nil, 0o600); err != nil {
			t.Fatalf("populate: %v", err)
		}
	}
	fs := mustFSProxy(t, &domainplugin.FSCaps{Read: []string{"${pluginData}"}}, dir)

	raw, err := fs.Handle("fs.list", mustJSON(map[string]any{"path": "."}))
	if err != nil {
		t.Fatalf("fs.list: %v", err)
	}
	var result struct {
		Entries []struct {
			Name string `json:"name"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// count generated files plus the seed file.
	if len(result.Entries) != count+1 {
		t.Errorf("listing returned %d entries, want %d", len(result.Entries), count+1)
	}
}

// offset + len(data) is int64 arithmetic on a number the plugin chose. An offset near MaxInt64
// wrapped to a negative sum, which passed the MaxFileBytes check and reached the seek — asking the
// filesystem for a file of exabytes.
func TestWriteRefusesAnOffsetThatWouldOverflowTheSizeCheck(t *testing.T) {
	dir := boundsProxyDir(t)
	fs := mustFSProxy(t, &domainplugin.FSCaps{Write: []string{"${pluginData}"}}, dir)
	payload := base64.StdEncoding.EncodeToString([]byte("boom"))

	for _, offset := range []int64{
		math.MaxInt64,
		math.MaxInt64 - 3,
		domainplugin.MaxFileBytes + 1,
	} {
		_, err := fs.Handle("fs.write", mustJSON(map[string]any{
			"path": "overflow.bin", "contentBase64": payload, "offset": offset,
		}))
		if !errors.Is(err, domainplugin.ErrCapabilityDenied) {
			t.Errorf("offset %d: err = %v, want ErrCapabilityDenied", offset, err)
		}
		if _, statErr := os.Stat(filepath.Join(dir, "overflow.bin")); statErr == nil {
			t.Fatalf("offset %d created a file", offset)
		}
	}
}

// The bound is on the offset itself, so an ordinary chunked write must still work.
func TestWriteStillAcceptsAnOffsetInsideTheFileLimit(t *testing.T) {
	dir := boundsProxyDir(t)
	fs := mustFSProxy(t, &domainplugin.FSCaps{Write: []string{"${pluginData}"}}, dir)

	if _, err := fs.Handle("fs.write", mustJSON(map[string]any{
		"path":          "chunked.bin",
		"contentBase64": base64.StdEncoding.EncodeToString([]byte("tail")),
		"offset":        int64(1024),
	})); err != nil {
		t.Fatalf("an offset of 1024 was refused: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "chunked.bin"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() != 1028 {
		t.Errorf("file size = %d, want 1028", info.Size())
	}
}
