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
