package plugin_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInfraHasNoSessionAuthorizationLogic(t *testing.T) {
	root := filepath.Join("..", "..", "..", "internal", "infra", "plugin")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		content := string(data)
		if strings.Contains(content, "AuthorizeSessionRPC") ||
			strings.Contains(content, "ScopedSessionProxy") ||
			strings.Contains(content, "enforceMultiSessionPolicy") {
			t.Fatalf("infra must not contain session authorization logic: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
