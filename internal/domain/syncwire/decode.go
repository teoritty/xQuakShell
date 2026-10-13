package syncwire

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"xquakshell/internal/domain/vaultsync"
)

// Decode reads exactly one JSON value of at most limit bytes into v, refusing unknown fields and
// anything after the value. Lenient decoding is how two implementations come to disagree about
// one message; this one has nothing to be lenient about.
func Decode(r io.Reader, limit int64, v any) error {
	lr := &io.LimitedReader{R: r, N: limit + 1}
	dec := json.NewDecoder(lr)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	if lr.N <= 0 {
		return fmt.Errorf("%w: body exceeds %d bytes", ErrMalformed, limit)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: data after the JSON value", ErrMalformed)
	}
	return nil
}

func parseID(s string) (vaultsync.ID, error) {
	id, err := vaultsync.ParseID(s)
	if err != nil {
		return vaultsync.ID{}, fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	return id, nil
}

// parseOptionalID accepts the empty string as the zero id, which is how an absent id travels.
func parseOptionalID(s string) (vaultsync.ID, error) {
	if s == "" {
		return vaultsync.ID{}, nil
	}
	return parseID(s)
}

func optionalID(id vaultsync.ID) string {
	if id.IsZero() {
		return ""
	}
	return id.String()
}

func parseHash(b []byte) ([32]byte, error) {
	var h [32]byte
	if len(b) != len(h) {
		return h, fmt.Errorf("%w: hash is %d bytes", ErrMalformed, len(b))
	}
	copy(h[:], b)
	return h, nil
}

func parseIDs(ss []string) ([]vaultsync.ID, error) {
	if ss == nil {
		return nil, nil
	}
	out := make([]vaultsync.ID, len(ss))
	for i, s := range ss {
		id, err := parseID(s)
		if err != nil {
			return nil, err
		}
		out[i] = id
	}
	return out, nil
}

func idStrings(ids []vaultsync.ID) []string {
	if ids == nil {
		return nil
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return out
}

// HexID renders an id for a URL path segment.
func HexID(id vaultsync.ID) string { return hex.EncodeToString(id[:]) }

// ParseID parses an id from a URL path segment or a message field.
func ParseID(s string) (vaultsync.ID, error) { return parseID(s) }
