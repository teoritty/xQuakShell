//go:build windows

package sandbox

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// appContainerMappings is where Windows records every AppContainer profile the current user owns.
// Each subkey is a container SID and carries a Moniker value holding the profile name — which is
// the only way to go from "what exists" back to "what we created", since the SID is derived from
// the name and not the other way round.
const appContainerMappings = `SOFTWARE\Classes\Local Settings\Software\Microsoft\Windows\CurrentVersion\AppContainer\Mappings`

// ListContainerNames returns the profile names registered for this user that start with prefix.
//
// A missing mappings key means no profile has ever been created on this account, which is an empty
// list rather than a failure.
func ListContainerNames(prefix string) ([]string, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, appContainerMappings, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		if err == registry.ErrNotExist {
			return nil, nil
		}
		return nil, fmt.Errorf("open app container mappings: %w", err)
	}
	defer func() { _ = key.Close() }()

	sids, err := key.ReadSubKeyNames(-1)
	if err != nil {
		return nil, fmt.Errorf("enumerate app container mappings: %w", err)
	}
	var names []string
	for _, sid := range sids {
		name, err := readMoniker(sid)
		if err != nil || !strings.HasPrefix(name, prefix) {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}

// readMoniker reads one mapping's profile name. A mapping that cannot be read is skipped rather
// than reported: this enumeration only ever feeds a cleanup, and one unreadable entry belonging to
// some other application must not stop it.
func readMoniker(sid string) (string, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, appContainerMappings+`\`+sid, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer func() { _ = key.Close() }()
	name, _, err := key.GetStringValue("Moniker")
	return name, err
}
