package agentforward

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// AuthorizedKeyBlob decodes the public key blob out of an authorized_keys line ("ssh-ed25519
// AAAA… comment"), which is how the vault caches a key's public half.
//
// Matching a sign request against the vault by blob means no private key is unwrapped to find
// out which key is being asked for — only the one that matched, and only after every other check
// has passed. The line's type and the blob's own format must agree, so a corrupted cache entry
// cannot make one key answer for another.
func AuthorizedKeyBlob(line string) ([]byte, error) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return nil, fmt.Errorf("%w: not an authorized_keys line", ErrInvalidConfig)
	}
	blob, err := base64.StdEncoding.DecodeString(fields[1])
	if err != nil {
		return nil, fmt.Errorf("%w: public key encoding", ErrInvalidConfig)
	}
	format, err := KeyFormat(blob)
	if err != nil || format != fields[0] {
		return nil, fmt.Errorf("%w: public key type disagrees with its blob", ErrInvalidConfig)
	}
	return blob, nil
}
