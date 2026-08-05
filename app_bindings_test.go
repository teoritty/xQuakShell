package main

import (
	"reflect"
	"sort"
	"testing"

	presentation "xquakshell/internal/presentation/wails"
)

// App is a hand-written facade that Wails binds; every AppAPI handler must have
// a delegate on it or the method silently disappears from the generated JS
// bindings and only fails at runtime in the browser. This test catches a
// forgotten delegate at build time instead.
//
// Handlers deliberately not exposed to the frontend go in notBound with a reason.
var notBound = map[string]string{
	// Composition-root wiring, called from main_*.go at startup.
	"SetContext":                     "wiring",
	"SetEmbedBridge":                 "wiring",
	"SetForwardRuleValidator":        "wiring",
	"SetGitHubServices":              "wiring",
	"SetPluginArbitraryNetworkGrant": "wiring",
	"SetPluginAuthGrant":             "wiring",
	"SetPluginManager":               "wiring",
	"SetPluginMultiSessionGrant":     "wiring",
	"SetPluginTunnelGrant":           "wiring",
	"SetPluginVaultGrant":            "wiring",
	"SetPluginViewRelay":             "wiring",
	"SetDiscoveryService":            "wiring",
	"SetSurfaceService":              "wiring",
	"SetDialogService":               "wiring",
	"SetNodeDetailsService":          "wiring",
	"Sessions":                       "wiring: accessor for the composition root",

	// Backend-initiated callbacks and lifecycle, never called from JS.
	"EmitPluginContributionsChanged": "backend-initiated event emitter",
	"EmitPluginStateChanged":         "backend-initiated event emitter",
	"EmitDiscoveryTreeChanged":       "backend-initiated event emitter",
	"EmitNodeDetailsChanged":         "backend-initiated event emitter",
	"HandlePluginViewMessage":        "implements usecase.PluginViewSink",
	"OnEmbedReady":                   "callback for embedTunnels.SetEmbedReadyHandler",
	"StopDebugLogWindow":             "lifecycle, driven from the backend",
	"SyncDebugLogWindow":             "lifecycle, driven from the backend",
	"Shutdown":                       "lifecycle, driven by Wails on App",
}

func exportedMethods(v any) []string {
	t := reflect.TypeOf(v)
	names := make([]string, 0, t.NumMethod())
	for i := 0; i < t.NumMethod(); i++ {
		names = append(names, t.Method(i).Name)
	}
	sort.Strings(names)
	return names
}

func TestAppExposesEveryAppAPIHandler(t *testing.T) {
	bound := make(map[string]bool)
	for _, n := range exportedMethods(&App{}) {
		bound[n] = true
	}

	for _, name := range exportedMethods(&presentation.AppAPI{}) {
		if reason, ok := notBound[name]; ok {
			t.Logf("AppAPI.%s intentionally not bound: %s", name, reason)
			continue
		}
		if !bound[name] {
			t.Errorf("AppAPI.%s has no delegate on App: it will be missing from the "+
				"Wails bindings and fail at runtime. Add a delegate in app.go, or "+
				"list it in notBound with a reason.", name)
		}
	}
}
