package locale

import (
	"encoding/json"
	"fmt"
	"strings"

	"xquakshell/internal/domain"
)

// Limits on what a language pack may contain.
//
// A pack is untrusted input that is parsed into memory and handed to the interface, so every
// dimension it could grow in has a ceiling. The numbers are far above a real translation — the
// English catalogue is on the order of 700 keys and tens of kilobytes — and far below anything
// that would hurt: the point is to refuse a file crafted to exhaust memory, not to police a
// translator who writes long sentences.
const (
	maxPackBytes = 1 << 20
	maxKeys      = 5000
	maxKeyBytes  = 200
	maxTextBytes = 4000
	maxNameBytes = 60
)

// packFile is the on-disk shape of a language pack.
//
// Unknown top-level fields are tolerated rather than rejected: a pack written for a later build
// that adds a field should still load here, and refusing it would make every future addition a
// breaking change for everyone who already published a translation.
type packFile struct {
	Code     string            `json:"code"`
	Name     string            `json:"name"`
	Messages map[string]string `json:"messages"`
}

// parsePack decodes and validates one pack, returning an error naming the first rule broken.
//
// A pack that breaks any rule is rejected whole rather than trimmed to its acceptable part. A
// half-loaded catalogue would leave the interface in a state no translator can reproduce or debug,
// and silently dropping the keys that were too long is worse than saying the file is wrong.
func parsePack(data []byte) (domain.LocalePack, error) {
	if len(data) > maxPackBytes {
		return domain.LocalePack{}, fmt.Errorf("locale pack is %d bytes, over the %d limit", len(data), maxPackBytes)
	}

	var file packFile
	// Decoding into map[string]string is itself the check that every value is a string: a nested
	// object, an array or a number fails here, so no separate shape walk is needed.
	if err := json.Unmarshal(data, &file); err != nil {
		return domain.LocalePack{}, fmt.Errorf("decode locale pack: %w", err)
	}

	if !ValidCode(file.Code) {
		return domain.LocalePack{}, fmt.Errorf("locale pack code %q: %w", file.Code, domain.ErrLocaleCodeInvalid)
	}
	if err := validateName(file.Name); err != nil {
		return domain.LocalePack{}, err
	}
	if err := validateMessages(file.Messages); err != nil {
		return domain.LocalePack{}, err
	}

	return domain.LocalePack{Code: file.Code, Name: file.Name, Messages: file.Messages}, nil
}

func validateName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("locale pack has no name")
	}
	if len(trimmed) > maxNameBytes {
		return fmt.Errorf("locale pack name is %d bytes, over the %d limit", len(trimmed), maxNameBytes)
	}
	return nil
}

func validateMessages(messages map[string]string) error {
	if len(messages) > maxKeys {
		return fmt.Errorf("locale pack has %d messages, over the %d limit", len(messages), maxKeys)
	}
	for key, text := range messages {
		if key == "" || len(key) > maxKeyBytes {
			return fmt.Errorf("locale pack message key %q is empty or over the %d byte limit", key, maxKeyBytes)
		}
		if len(text) > maxTextBytes {
			return fmt.Errorf("locale pack message %q is %d bytes, over the %d limit", key, len(text), maxTextBytes)
		}
	}
	return nil
}
