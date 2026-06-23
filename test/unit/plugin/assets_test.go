package plugin_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	infrapluginassets "ssh-client/internal/infra/plugin/assets"
)

func TestHandlerServesPluginFile(t *testing.T) {
	root := t.TempDir()
	html := filepath.Join(root, "ui", "index.html")
	if err := os.MkdirAll(filepath.Dir(html), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(html, []byte("<html>ok</html>"), 0600); err != nil {
		t.Fatal(err)
	}

	handler := infrapluginassets.NewHandler(func(pluginID string) (string, error) {
		if pluginID != "com.test.plugin" {
			t.Fatalf("unexpected plugin id %s", pluginID)
		}
		return filepath.Join(root, "ui"), nil
	})

	req := httptest.NewRequest(http.MethodGet, "/plugin/com.test.plugin/ui/index.html", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" || strings.Contains(csp, "'unsafe-inline'") {
		t.Fatalf("expected strict CSP without unsafe-inline, got %q", csp)
	}
}

func TestHandlerRejectsEngineBinary(t *testing.T) {
	root := t.TempDir()
	ui := filepath.Join(root, "ui")
	if err := os.MkdirAll(ui, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "plugin.exe"), []byte("MZ"), 0600); err != nil {
		t.Fatal(err)
	}

	handler := infrapluginassets.NewHandler(func(string) (string, error) {
		return ui, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/plugin/com.test.plugin/../plugin.exe", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusForbidden {
		t.Fatalf("expected rejection, got %d", rec.Code)
	}
}

func TestHandlerRejectsPluginJSON(t *testing.T) {
	root := t.TempDir()
	ui := filepath.Join(root, "ui")
	if err := os.MkdirAll(ui, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ui, "plugin.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}

	handler := infrapluginassets.NewHandler(func(string) (string, error) {
		return ui, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/plugin/com.test.plugin/plugin.json", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden, got %d", rec.Code)
	}
}

func TestHandlerRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	handler := infrapluginassets.NewHandler(func(string) (string, error) {
		return filepath.Join(root, "ui"), nil
	})

	req := httptest.NewRequest(http.MethodGet, "/plugin/com.test.plugin/../secret.txt", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusForbidden {
		t.Fatalf("expected rejection, got %d", rec.Code)
	}
}
