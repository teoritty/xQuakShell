package plugin

import (
	"encoding/json"
	"fmt"
)

const XQSPManifestFile = "xqsp.json"

// XQSPManifest is the GitHub repository plugin manifest format.
type XQSPManifest struct {
	Manifest
	Author   string   `json:"author,omitempty"`
	License  string   `json:"license,omitempty"`
	Homepage string   `json:"homepage,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

// ParseXQSPManifest parses xqsp.json content into manifest and metadata fields.
func ParseXQSPManifest(data []byte) (XQSPManifest, error) {
	var m XQSPManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return XQSPManifest{}, fmt.Errorf("parse xqsp.json: %w", err)
	}
	if err := m.Validate(); err != nil {
		return XQSPManifest{}, err
	}
	return m, nil
}
