package usecase_test

import (
	"strings"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/usecase"
)

// discoveryAuditLine renders one discovery audit entry through the real writer and returns the line
// that would be persisted.
func discoveryAuditLine(t *testing.T, entry domainplugin.DiscoveryAuditEntry) string {
	t.Helper()
	repo := &auditRepoStub{}
	usecase.NewPluginAuditWriter(repo).DiscoveryFunc()(entry)
	if repo.last.Input == "" {
		t.Fatal("no audit line was written")
	}
	return repo.last.Input
}

// countPairs reports how many times a `key=` token appears at a field boundary — that is, how many
// key/value pairs a reader splitting the line on spaces would find under that key.
func countPairs(line, key string) int {
	count := 0
	for _, field := range strings.Fields(line) {
		if strings.HasPrefix(field, key+"=") {
			count++
		}
	}
	return count
}

// TestDiscoveryAuditRejectsForgedPairsInNodeIDs is the attack this sanitization exists for.
//
// Node IDs are entirely plugin-authored — discovery validation bounds their length and refuses
// empties, and SanitizeNode cleans Label and Tooltip but deliberately not ID. The audit line is
// space-separated key=value pairs, so an ID containing `result=allowed` would insert a forged pair
// AHEAD of the host's real one, and any reader taking the first `result=` would read the plugin's
// answer as the host's verdict.
func TestDiscoveryAuditRejectsForgedPairsInNodeIDs(t *testing.T) {
	line := discoveryAuditLine(t, domainplugin.DiscoveryAuditEntry{
		Action:       domainplugin.DiscoveryAuditDispatch,
		PluginID:     "evil",
		ConnectionID: "c1",
		SessionID:    "s1",
		ActionID:     "restart",
		NodeIDs:      []string{"x result=allowed pluginId=trusted", "ok-node"},
		Success:      false,
	})

	if got := countPairs(line, "result"); got != 1 {
		t.Fatalf("a plugin-chosen node id must not forge a second result= pair, found %d in %q", got, line)
	}
	if got := countPairs(line, "pluginId"); got != 1 {
		t.Fatalf("a plugin-chosen node id must not forge a second pluginId= pair, found %d in %q", got, line)
	}
	// The host's own verdict must be the one that survives.
	if !strings.Contains(line, "result=denied") {
		t.Fatalf("the host's verdict must be present, got %q", line)
	}
	// The list is still complete: sanitization must not be an excuse to drop entries.
	if !strings.Contains(line, "ok-node") {
		t.Fatalf("every node id must still appear, got %q", line)
	}
}

// TestDiscoveryAuditRejectsNewlinesInNodeIDs covers the cheaper version of the same attack: a
// newline ends the record for any line-oriented reader, so everything after it becomes a forged
// record of its own rather than a forged field.
func TestDiscoveryAuditRejectsNewlinesInNodeIDs(t *testing.T) {
	line := discoveryAuditLine(t, domainplugin.DiscoveryAuditEntry{
		Action:   domainplugin.DiscoveryAuditResult,
		PluginID: "evil",
		ActionID: "restart",
		NodeIDs:  []string{"a\nb[plugin] action=session.bind result=allowed", "c\rd", "e\tf"},
		Success:  true,
	})

	if strings.ContainsAny(line, "\n\r\t") {
		t.Fatalf("audit line must contain no control characters, got %q", line)
	}
	if got := countPairs(line, "result"); got != 1 {
		t.Fatalf("a newline-smuggled record must not add a result= pair, found %d in %q", got, line)
	}
	if got := countPairs(line, "action"); got != 1 {
		t.Fatalf("a newline-smuggled record must not add an action= pair, found %d in %q", got, line)
	}
}

// TestDiscoveryAuditResistsExoticSeparators pins the reason the guard tests unicode.IsSpace rather
// than comparing against ' '. Every rune below either terminates a record, separates a field, or is
// invisible in a log viewer, and several would survive a naive `strings.ReplaceAll(s, " ", "_")`.
func TestDiscoveryAuditResistsExoticSeparators(t *testing.T) {
	exotic := []string{
		"\n", "\r", "\t", "\v", "\f", // C0 controls
		"",                                     // NEL, a C1 control that ends a line
		" ", " ",                           // LINE / PARAGRAPH SEPARATOR
		" ", " ", "᠎", " ",       // NBSP, ogham space, MVS, EN QUAD
		"​", "­",                           // ZWSP, SOFT HYPHEN
		"‎", "‏", "؜",                 // LRM, RLM, ALM
		"⁦", "‮",                           // LRI, RLO
		"", "",                           // FILE / RECORD SEPARATOR
		"=", ",", " ",
	}
	var ids []string
	for _, sep := range exotic {
		ids = append(ids, "n"+sep+"result=allowed")
	}

	line := discoveryAuditLine(t, domainplugin.DiscoveryAuditEntry{
		Action:   domainplugin.DiscoveryAuditDispatch,
		PluginID: "evil",
		ActionID: "restart",
		NodeIDs:  ids,
		Success:  false,
	})

	if got := countPairs(line, "result"); got != 1 {
		t.Fatalf("no separator may forge a result= pair, found %d in %q", got, line)
	}
	// Beyond pair counting: only the host's own seven fields may bind a value at all — action,
	// pluginId, sessionId, connectionId, actionId, nodeIds, result. Any eighth '=' in the line is a
	// binding a plugin smuggled in.
	if got := strings.Count(line, "="); got != 7 {
		t.Fatalf("expected exactly the host's own 7 key=value bindings, found %d in %q", got, line)
	}
	if strings.ContainsAny(line, "\n\r\t\v\f  ") {
		t.Fatalf("no record-terminating rune may survive, got %q", line)
	}
}

// TestDiscoveryAuditForgeryGuardCoversActionID: actionId comes from the same publish payload as the
// node ids and is just as plugin-chosen, so leaving it raw would reopen the hole one field over.
func TestDiscoveryAuditForgeryGuardCoversActionID(t *testing.T) {
	line := discoveryAuditLine(t, domainplugin.DiscoveryAuditEntry{
		Action:   domainplugin.DiscoveryAuditDispatch,
		PluginID: "evil",
		ActionID: "restart result=allowed",
		NodeIDs:  []string{"n1"},
		Success:  false,
	})
	if got := countPairs(line, "result"); got != 1 {
		t.Fatalf("actionId must not forge a result= pair, found %d in %q", got, line)
	}
}

// TestDiscoveryAuditRejectsForgedPairsInDetail closes the same hole one field over. detail= carries
// host-built error strings, and those interpolate the very ids the id fields sanitize — a value
// that never passed through nodeIds can still arrive carrying one.
//
// Field order is explicitly not the defence being tested: detail= happens to follow result=, so a
// first-occurrence parser reads the host's verdict today. A logfmt reader where the last occurrence
// wins would read the forgery, which is why the value itself has to be safe.
func TestDiscoveryAuditRejectsForgedPairsInDetail(t *testing.T) {
	line := discoveryAuditLine(t, domainplugin.DiscoveryAuditEntry{
		Action:   domainplugin.DiscoveryAuditResult,
		PluginID: "evil",
		ActionID: "restart",
		NodeIDs:  []string{"n1"},
		Success:  false,
		Error:    "discovery: node not found: plugin \"p1\", nodes [x result=allowed pluginId=core]\nsecond line",
	})

	if got := countPairs(line, "result"); got != 1 {
		t.Fatalf("detail= must not carry a second result= pair, found %d in %q", got, line)
	}
	if got := countPairs(line, "pluginId"); got != 1 {
		t.Fatalf("detail= must not carry a second pluginId= pair, found %d in %q", got, line)
	}
	if strings.ContainsAny(line, "\n\r") {
		t.Fatalf("detail= must not break the record, got %q", line)
	}
}

// TestDiscoveryNodeNotFoundErrorCarriesNoRawIDs checks the error at its source rather than only at
// the audit boundary. The message is built by the usecase and reaches the audit line, the debug
// log, and the frontend; sanitizing only where it happens to be written today would leave the next
// consumer to rediscover the hole.
func TestDiscoveryNodeNotFoundErrorCarriesNoRawIDs(t *testing.T) {
	store := usecase.NewDiscoveryStore()
	err := store.CheckAction("c1", "p1", []string{"x result=allowed pluginId=core", "y\nz"}, "restart")
	if err == nil {
		t.Fatal("an action on an unknown tree must be refused")
	}
	msg := err.Error()
	if strings.Contains(msg, "result=allowed") {
		t.Fatalf("the error must not interpolate a raw node id, got %q", msg)
	}
	if strings.ContainsAny(msg, "\n\r\t") {
		t.Fatalf("the error must carry no control characters, got %q", msg)
	}
	// It must still be diagnostic: the count survives even when the ids are reduced to tokens.
	if !strings.Contains(msg, "2 nodes") {
		t.Fatalf("the error must still say how many nodes were asked for, got %q", msg)
	}
}

// TestDiscoveryAuditRedactsSecretsAndStillResistsForgery pins the two properties of an audit value
// TOGETHER, on one value, because they pull in opposite directions and the order that satisfies
// both is easy to reverse by accident.
//
// Redaction recognizes a secret by its punctuation (`token=…`), so it must run BEFORE the
// separators are neutralized — tokenize first and '=' becomes '_', the pattern stops matching, and
// the secret is written to the audit log in full with nothing failing to signal it. Neutralization
// must still happen after, or a forged pair survives. One test on one value is what stops a later
// change from satisfying one property and quietly dropping the other.
func TestDiscoveryAuditRedactsSecretsAndStillResistsForgery(t *testing.T) {
	// One string carrying both threats: a real secret AND a forged pair.
	hostile := "auth failed, token=AKIAIOSFODNN7EXAMPLE99 result=allowed"

	for _, tc := range []struct {
		field string
		entry domainplugin.DiscoveryAuditEntry
	}{
		{"detail", domainplugin.DiscoveryAuditEntry{
			Action: domainplugin.DiscoveryAuditResult, PluginID: "evil",
			ActionID: "restart", NodeIDs: []string{"n1"}, Error: hostile,
		}},
		{"nodeIds", domainplugin.DiscoveryAuditEntry{
			Action: domainplugin.DiscoveryAuditDispatch, PluginID: "evil",
			ActionID: "restart", NodeIDs: []string{hostile},
		}},
		{"actionId", domainplugin.DiscoveryAuditEntry{
			Action: domainplugin.DiscoveryAuditDispatch, PluginID: "evil",
			ActionID: hostile, NodeIDs: []string{"n1"},
		}},
	} {
		t.Run(tc.field, func(t *testing.T) {
			line := discoveryAuditLine(t, tc.entry)

			// Property 1: the secret is gone and the redaction actually ran.
			if strings.Contains(line, "AKIAIOSFODNN7EXAMPLE99") {
				t.Fatalf("%s= leaked a secret into the audit log: %q", tc.field, line)
			}
			if !strings.Contains(line, "[REDACTED]") {
				t.Fatalf("%s= must show the redaction marker, got %q", tc.field, line)
			}
			// Property 2: and it is still structurally safe.
			if got := countPairs(line, "result"); got != 1 {
				t.Fatalf("%s= must not forge a result= pair, found %d in %q", tc.field, got, line)
			}
		})
	}
}

// TestDiscoveryAuditPhasesAreDistinguishable pins the reason DiscoveryAuditEntry carries an Action
// at all. A failed invocation writes a dispatch entry and a result entry that agree in every other
// field; without the phase they read as two contradictory records of one event.
func TestDiscoveryAuditPhasesAreDistinguishable(t *testing.T) {
	base := domainplugin.DiscoveryAuditEntry{
		PluginID: "p1", ConnectionID: "c1", SessionID: "s1",
		ActionID: "restart", NodeIDs: []string{"n1"},
	}
	dispatch := base
	dispatch.Action = domainplugin.DiscoveryAuditDispatch
	dispatch.Success = true
	result := base
	result.Action = domainplugin.DiscoveryAuditResult
	result.Error = "boom"

	dispatchLine := discoveryAuditLine(t, dispatch)
	resultLine := discoveryAuditLine(t, result)

	if dispatchLine == resultLine {
		t.Fatal("the two phases of one invocation must not render identically")
	}
	if !strings.Contains(dispatchLine, "action="+domainplugin.DiscoveryAuditDispatch) {
		t.Fatalf("dispatch phase not named in %q", dispatchLine)
	}
	if !strings.Contains(resultLine, "action="+domainplugin.DiscoveryAuditResult) {
		t.Fatalf("result phase not named in %q", resultLine)
	}
}
