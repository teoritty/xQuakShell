package domain

import "fmt"

// ConnectionFolder groups connections into a named folder.
// ParentID is empty for root-level folders.
type ConnectionFolder struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ParentID string `json:"parentId"`
	Order    int    `json:"order"`
}

// Validate checks that the folder has a non-empty name.
// Returns ErrInvalidConnectionConfig wrapped with context on failure.
func (f *ConnectionFolder) Validate() error {
	if f.Name == "" {
		return fmt.Errorf("folder name must not be empty: %w", ErrInvalidConnectionConfig)
	}
	return nil
}
