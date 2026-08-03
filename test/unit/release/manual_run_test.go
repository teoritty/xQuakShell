package release_test

import (
	"strings"
	"testing"
)

// The release workflow can be started by hand, so a build can be proven to still work before a tag
// exists — a tag cannot be taken back cleanly and a published release is public immediately.
//
// That convenience is only safe as long as a manual run cannot publish. On workflow_dispatch there
// is no tag: github.ref_name is the branch name, and an unguarded publish job would take the
// archives it produced and cut a release called "main".
func TestPublishRefusesToRunOutsideATag(t *testing.T) {
	workflow := readReleaseWorkflow(t)
	if !strings.Contains(workflow, "workflow_dispatch") {
		return // no manual trigger, so nothing can reach publish except a tag
	}
	if !strings.Contains(workflow, "if: github.ref_type == 'tag'") {
		t.Error("release.yml can be triggered manually but the publish job is not restricted to " +
			"tags; a manual run would publish a release named after the branch")
	}
}

// A manual run must also not produce archives that look like a release build. The version is
// stamped into the binary and into every archive name, and "xQuakShell-main-…" sitting next to a
// real download is a support problem waiting to happen.
func TestManualRunsAreNamedAsDryRuns(t *testing.T) {
	workflow := readReleaseWorkflow(t)
	if !strings.Contains(workflow, "workflow_dispatch") {
		return
	}
	if !strings.Contains(workflow, "dryrun-") {
		t.Error("release.yml can be triggered manually but never renames the build; the archives " +
			"would be named after the branch and be indistinguishable from a release build")
	}
	if strings.Count(workflow, "github.ref_type") < 3 {
		t.Error("not every job distinguishes a tag from a manual run; one of them would still " +
			"name its archives after the branch")
	}
}
