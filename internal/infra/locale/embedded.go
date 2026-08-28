package locale

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"xquakshell/internal/domain"
)

// builtinFS carries the languages that ship inside the binary. They are the trust anchor for the
// security namespace and the fallback for every key an on-disk pack leaves out, so they must be
// present no matter what the filesystem next to the executable looks like — including a read-only
// or entirely absent data directory.
//
//go:embed builtin/*.json
var builtinFS embed.FS

// loadBuiltins parses every embedded pack once, at construction.
//
// A parse failure here is a build defect rather than a runtime condition — the files are compiled
// in — so it is returned to the composition root instead of being skipped the way a bad on-disk
// pack is.
func loadBuiltins() (map[string]domain.LocalePack, error) {
	entries, err := fs.ReadDir(builtinFS, "builtin")
	if err != nil {
		return nil, fmt.Errorf("read embedded locales: %w", err)
	}

	packs := make(map[string]domain.LocalePack, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		data, err := builtinFS.ReadFile(path.Join("builtin", name))
		if err != nil {
			return nil, fmt.Errorf("read embedded locale %s: %w", name, err)
		}
		pack, err := parsePack(data)
		if err != nil {
			return nil, fmt.Errorf("embedded locale %s: %w", name, err)
		}
		packs[pack.Code] = pack
	}

	if _, ok := packs[domain.DefaultLocale]; !ok {
		return nil, fmt.Errorf("embedded locales are missing %q, which every other language falls back to", domain.DefaultLocale)
	}
	return packs, nil
}
