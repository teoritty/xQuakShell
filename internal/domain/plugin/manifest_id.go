package plugin

import (
	"fmt"
	"regexp"
	"strings"
)

var manifestIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,127}$`)

// ValidateID checks that a plugin id is safe for filesystem and IPC use.
func ValidateID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidManifest)
	}
	if strings.Contains(id, "..") || strings.ContainsAny(id, `/\`) {
		return fmt.Errorf("%w: id contains invalid path characters", ErrInvalidManifest)
	}
	if !manifestIDPattern.MatchString(id) {
		return fmt.Errorf("%w: id must match reverse-DNS pattern [a-z0-9][a-z0-9.-]{1,127}", ErrInvalidManifest)
	}
	return nil
}
