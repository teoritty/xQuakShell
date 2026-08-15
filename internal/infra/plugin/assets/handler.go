package assets

import (
	"net/http"
	"path/filepath"
	"strings"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/pkg/pathsafe"
)

const pluginAssetPrefix = "/plugin/"

// UIRootResolver returns the ui/ directory for a plugin ID.
type UIRootResolver func(pluginID string) (string, error)

// Handler serves static plugin UI assets from registered plugin ui/ directories.
type Handler struct {
	resolve UIRootResolver
}

// NewHandler creates a plugin asset HTTP handler.
func NewHandler(resolve UIRootResolver) *Handler {
	return &Handler{resolve: resolve}
}

// ServeHTTP implements http.Handler for /plugin/<pluginId>/<path> requests.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.resolve == nil {
		http.NotFound(w, r)
		return
	}

	path := r.URL.Path
	if !strings.HasPrefix(path, pluginAssetPrefix) {
		http.NotFound(w, r)
		return
	}

	rest := strings.TrimPrefix(path, pluginAssetPrefix)
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		http.NotFound(w, r)
		return
	}

	pluginID := parts[0]
	rel, err := ResolveUIRelPath(parts[1])
	if err != nil {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	if !IsAllowedAssetName(filepath.Base(rel)) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	uiRoot, err := h.resolve(pluginID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	absRoot, err := filepath.Abs(uiRoot)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	target := filepath.Join(absRoot, rel)
	absTarget, err := filepath.Abs(target)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if !pathsafe.UnderRoot(absRoot, absTarget) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// 'wasm-unsafe-eval' is what lets a plugin UI instantiate a .wasm module it fetched, and it is
	// the narrow opt-in rather than the old 'unsafe-eval': it permits WebAssembly compilation and
	// nothing else - no eval, no Function(), no inline script. The embed broker already runs with
	// exactly this, so the two surfaces agree. Without it the allowlisted .wasm file is served and
	// then refused by the renderer, which is the worst of both: shipped and silently inert.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self'; img-src 'self' data:; frame-ancestors 'none'")

	// The MIME type is set here rather than left to http.ServeContent. On Windows the type table
	// is read from the registry, where any installer may have claimed .wasm - and a wrong type
	// makes WebAssembly.instantiateStreaming refuse a file that downloaded perfectly.
	if strings.EqualFold(filepath.Ext(rel), ".wasm") {
		w.Header().Set("Content-Type", "application/wasm")
	}

	resolved, err := pathsafe.SecurePathUnderRoots(absTarget, []string{absRoot})
	if err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	f, err := pathsafe.OpenExistingFile([]string{absRoot}, resolved, false)
	if err != nil {
		if pathsafe.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

// PluginRegistryUIRootResolver adapts a registry get func to the ui/ sandbox root.
func PluginRegistryUIRootResolver(get func(id string) (domainplugin.InstalledPlugin, error)) UIRootResolver {
	return func(pluginID string) (string, error) {
		p, err := get(pluginID)
		if err != nil {
			return "", err
		}
		return filepath.Join(p.RootDir, "ui"), nil
	}
}
