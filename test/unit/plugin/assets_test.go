package plugin_test

import (
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	infrapluginassets "xquakshell/internal/infra/plugin/assets"
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

// A .wasm asset has three moving parts - the extension allowlist, the CSP and the MIME type - and
// two of them are invisible in a manual click-through: the file downloads and the module simply
// never instantiates. All three are asserted here, together, because any one of them alone ships
// something inert.
func TestHandlerServesWasmInstantiably(t *testing.T) {
	root := t.TempDir()
	ui := filepath.Join(root, "ui")
	if err := os.MkdirAll(ui, 0700); err != nil {
		t.Fatal(err)
	}
	// The WebAssembly magic number and version. Not a valid module, but ServeContent must not be
	// sniffing content in the first place.
	if err := os.WriteFile(filepath.Join(ui, "app.wasm"), []byte("\x00asm\x01\x00\x00\x00"), 0600); err != nil {
		t.Fatal(err)
	}

	// The hostile environment the explicit header exists for: on Windows the MIME table is read
	// from the registry, where an installer may have claimed .wasm. Simulated here because a
	// machine where the stdlib already answers correctly cannot tell whether the handler set the
	// type or merely inherited it - and the whole point is that it must not depend on inheriting.
	if err := mime.AddExtensionType(".wasm", "text/plain"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mime.AddExtensionType(".wasm", "application/wasm"); err != nil {
			t.Fatal(err)
		}
	})

	handler := infrapluginassets.NewHandler(func(string) (string, error) { return ui, nil })

	req := httptest.NewRequest(http.MethodGet, "/plugin/com.test.plugin/app.wasm", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/wasm" {
		t.Fatalf("Content-Type = %q, want application/wasm; instantiateStreaming refuses anything else", ct)
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "'wasm-unsafe-eval'") {
		t.Fatalf("CSP = %q, want 'wasm-unsafe-eval'; without it the served module cannot be instantiated", csp)
	}
	if strings.Contains(csp, "'unsafe-eval'\"") || strings.Contains(csp, " 'unsafe-eval'") {
		t.Fatalf("CSP = %q; wasm must not be bought with full eval", csp)
	}
	if strings.Contains(csp, "'unsafe-inline'") {
		t.Fatalf("CSP = %q; the wasm opt-in must not have loosened anything else", csp)
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
