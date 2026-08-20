package locale

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"xquakshell/internal/domain"
	"xquakshell/internal/pkg/pathsafe"
)

// DirName is the folder under the portable data root holding user-supplied language packs.
const DirName = "locales"

// diskReader enumerates and reads language packs from one directory.
type diskReader struct {
	root string
	log  *slog.Logger
}

// list returns the packs currently on disk, keyed by language code.
//
// The directory is re-read on every call rather than cached. A pack is a file a user drops next to
// the executable and then edits, and the behaviour they expect is that reopening the language list
// shows what the folder now contains. The files are small and the call is rare — it happens when
// the settings dialog opens, not per frame.
func (r *diskReader) list() map[string]domain.LocalePack {
	packs := map[string]domain.LocalePack{}
	if r.root == "" {
		return packs
	}

	entries, err := os.ReadDir(r.root)
	if err != nil {
		// A missing folder is the normal case: most installations have no user packs at all, and
		// the data root may legitimately be read-only (a USB stick, an installed copy).
		if !os.IsNotExist(err) {
			r.log.Warn("read locales directory", "dir", r.root, "err", err)
		}
		return packs
	}

	for _, entry := range entries {
		code, ok := packCode(entry)
		if !ok {
			continue
		}
		pack, err := r.read(code)
		if err != nil {
			r.log.Warn("skip locale pack", "code", code, "err", err)
			continue
		}
		packs[pack.Code] = pack
	}
	return packs
}

// read loads one pack by its language code.
//
// The code is validated before it is joined into a path, and pathsafe re-checks containment on the
// opened descriptor. Both are needed: the first refuses a malformed code outright, the second
// refuses a well-formed one whose file turns out to be a symlink pointing out of the directory,
// which no amount of string checking can see.
func (r *diskReader) read(code string) (domain.LocalePack, error) {
	if !domain.ValidLocaleCode(code) {
		return domain.LocalePack{}, domain.ErrLocaleCodeInvalid
	}
	full := filepath.Join(r.root, code+".json")
	data, err := pathsafe.ReadExistingFile([]string{r.root}, full, maxPackBytes)
	if err != nil {
		return domain.LocalePack{}, err
	}
	pack, err := parsePack(data)
	if err != nil {
		return domain.LocalePack{}, err
	}
	// A pack whose declared code disagrees with its filename is refused rather than resolved in
	// favour of one of them: the file name decides which language the user gets by dropping it in,
	// and letting the contents claim a different one makes de.json quietly replace English.
	if pack.Code != code {
		return domain.LocalePack{}, domain.ErrLocaleCodeInvalid
	}
	return pack, nil
}

// packCode returns the language code a directory entry stands for, if it is a pack at all.
//
// Only regular files directly in the folder qualify. Subdirectories are not descended into and
// symlinks are not followed, so the set of files considered is exactly the set a user can see.
func packCode(entry os.DirEntry) (string, bool) {
	if !entry.Type().IsRegular() {
		return "", false
	}
	name := entry.Name()
	if !strings.HasSuffix(name, ".json") {
		return "", false
	}
	code := strings.TrimSuffix(name, ".json")
	if !domain.ValidLocaleCode(code) {
		return "", false
	}
	return code, true
}
