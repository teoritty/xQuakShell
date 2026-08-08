package release_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// nightly.yml is a copy of release.yml's build steps, kept separate so the release path is never
// edited to serve the nightly. A copy drifts, and a nightly that builds something other than what
// the release builds stops being a rehearsal of it - it becomes a second, untested pipeline whose
// green runs prove nothing. These assertions are what makes the copy safe: they fail when the two
// files stop agreeing on the things a build's output depends on.

func readNightlyWorkflow(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(mustRepoRoot(t), ".github", "workflows", "nightly.yml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// jobSection returns the portion of a workflow belonging to one top-level job. Job keys sit at two
// spaces of indentation and everything inside them deeper, so the next key at that depth ends it.
func jobSection(t *testing.T, workflow, job string) string {
	t.Helper()
	start := strings.Index(workflow, "\n  "+job+":")
	if start < 0 {
		t.Fatalf("workflow has no %s job", job)
	}
	rest := workflow[start+1:]
	if end := regexp.MustCompile(`\n  [a-z][\w-]*:`).FindStringIndex(rest); end != nil {
		return rest[:end[0]]
	}
	return rest
}

// withoutComments drops whole-line YAML and shell comments. An assertion that a command is absent
// has to read the commands only: the comment explaining why that command is the wrong one to use
// mentions it by name, and would otherwise be indistinguishable from using it.
func withoutComments(text string) string {
	var kept []string
	for line := range strings.SplitSeq(text, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

// sourcesCopied lists the first capture of every match, sorted, so two jobs can be compared on the
// set of files they put into an archive rather than on the order of their steps.
func sourcesCopied(pattern *regexp.Regexp, job string) []string {
	var found []string
	for _, match := range pattern.FindAllStringSubmatch(job, -1) {
		found = append(found, match[1])
	}
	sort.Strings(found)
	return found
}

// The schedule is the whole feature: 04:00 Moscow time, which is 01:00 UTC because MSK has been a
// fixed UTC+3 since 2014. A cron edited to a "nicer" hour would move the build silently, so the
// expression is asserted exactly rather than merely being present.
func TestNightlyRunsAtFourMoscowTime(t *testing.T) {
	workflow := readNightlyWorkflow(t)
	if !strings.Contains(workflow, "cron: '0 1 * * *'") {
		t.Error("nightly.yml no longer schedules 01:00 UTC; the build would not run at 04:00 MSK")
	}
}

// A nightly that runs on an unchanged main publishes a rebuild of a release that already exists and
// pushes the previous one out of the list for nothing. The decide job answers that question, and
// every job that costs runner minutes or writes a release hangs off its answer.
func TestNightlySkipsWhenMainHasNotMoved(t *testing.T) {
	workflow := readNightlyWorkflow(t)
	decide := jobSection(t, workflow, "decide")

	// The comparison is against the commit the published nightly was built from, not against a
	// time window: that is what lets a failed night be picked up by the next one.
	if !strings.Contains(decide, "refs/tags/nightly") {
		t.Error("the decide job never resolves the nightly tag, so it cannot tell whether main moved")
	}
	if !strings.Contains(decide, "gh release view nightly") {
		t.Error("the decide job never checks that a nightly release exists; a tag left behind by a " +
			"half-finished run would be read as proof the build already shipped")
	}

	for _, job := range []string{"windows", "linux"} {
		if !strings.Contains(jobSection(t, workflow, job), "needs.decide.outputs.build == 'true'") {
			t.Errorf("the %s job is not gated on the decide job; it would build on an unchanged main", job)
		}
	}
}

// "Always exactly one nightly" is enforced by deleting before creating, at a tag name that never
// changes. A dated tag, or a create without the delete, leaves the release list growing by one
// entry a day - the thing this workflow exists to avoid.
func TestNightlyReplacesThePreviousRelease(t *testing.T) {
	publish := jobSection(t, readNightlyWorkflow(t), "publish")

	if !strings.Contains(publish, `select(.tag_name == "nightly") | .id`) {
		t.Error("the publish job does not enumerate the existing nightly releases by id; a draft " +
			"left by a half-finished run has no tag to be found by and would accumulate")
	}
	if !strings.Contains(publish, `gh api -X DELETE "repos/${GITHUB_REPOSITORY}/releases/${id}"`) {
		t.Error("the publish job does not delete the previous nightly release")
	}
	if !strings.Contains(publish, "tag_name: nightly") {
		t.Error("the published release is not pinned to the fixed nightly tag, so its download " +
			"links would move every night")
	}

	deleted, created := strings.Index(publish, "-X DELETE"), strings.Index(publish, "action-gh-release")
	if deleted > created {
		t.Error("the delete runs after the release is created, which would delete the new one")
	}

	// The tag must outlive the release it belongs to by exactly nothing: deleting it first demotes
	// the still-present release to a draft, which is invisible to every user it was built for.
	release := strings.Index(publish, `/releases/${id}"`)
	tag := strings.Index(publish, "/git/refs/tags/nightly")
	if release < 0 || tag < 0 || release > tag {
		t.Error("the nightly tag is deleted before the release that points at it, which turns the " +
			"release into a draft instead of removing it")
	}
}

// This job has no checkout, and gh infers the repository it acts on from a git remote. Without
// GH_REPO every gh call in it fails on a directory that is not a repository - which is how the
// delete above once became a no-op, silently turning "replace the nightly" into "update it".
func TestNightlyDeletionDoesNotDependOnACheckout(t *testing.T) {
	publish := jobSection(t, readNightlyWorkflow(t), "publish")

	if !strings.Contains(publish, "GH_REPO: ${{ github.repository }}") {
		t.Error("the publish job runs gh without GH_REPO and without a checkout; gh cannot tell " +
			"which repository to act on")
	}
	// `gh release delete --cleanup-tag` shells out to git for the tag half of its work, so it
	// cannot do this job's work at all. The API calls it was replaced with need no working tree.
	if strings.Contains(withoutComments(publish), "gh release delete") {
		t.Error("the publish job uses gh release delete, which needs a git working tree it does not have")
	}
	// A tolerated failure here is indistinguishable from a successful delete, and the difference
	// between them is whether the workflow does the one thing it exists to do.
	if regexp.MustCompile(`gh api -X DELETE "repos/\$\{GITHUB_REPOSITORY\}/releases/\$\{id\}"\s*\|\|`).MatchString(publish) {
		t.Error("a failed release delete is swallowed; the run would then update the old release " +
			"in place and report success")
	}
}

// The default only applies to a release the action creates. On the update path an inherited draft
// flag survives, and a drafted nightly is published to nobody.
func TestNightlyIsNeverPublishedAsADraft(t *testing.T) {
	publish := jobSection(t, readNightlyWorkflow(t), "publish")

	if !strings.Contains(publish, "draft: false") {
		t.Error("the nightly release does not state draft: false")
	}
}

// The green "Latest" badge and the /releases/latest redirect are what a first-time visitor follows.
// A nightly must never claim either, or an untested build becomes the default download.
func TestNightlyIsNeverTheLatestRelease(t *testing.T) {
	publish := jobSection(t, readNightlyWorkflow(t), "publish")

	for _, flag := range []string{"prerelease: true", "make_latest: false"} {
		if !strings.Contains(publish, flag) {
			t.Errorf("the nightly release does not set %q; it would be offered as the current release", flag)
		}
	}
}

// Two CLIs produce two different binaries from the same source, and a nightly built by a CLI the
// release does not use tests a toolchain nobody ships. The pin is asserted against release.yml
// rather than against a literal so a Dependabot bump has one place to be wrong, not two.
func TestNightlyPinsTheSameWailsCLIAsTheRelease(t *testing.T) {
	pins := regexp.MustCompile(`cmd/wails@v[\d.]+`)

	want := pins.FindString(readReleaseWorkflow(t))
	if want == "" {
		t.Fatal("release.yml pins no Wails CLI version; the nightly has nothing to match")
	}
	found := pins.FindAllString(readNightlyWorkflow(t), -1)
	if len(found) == 0 {
		t.Fatal("nightly.yml installs no pinned Wails CLI")
	}
	for _, pin := range found {
		if pin != want {
			t.Errorf("nightly.yml pins %q, release.yml pins %q; the two would build with "+
				"different toolchains", pin, want)
		}
	}
}

// Without the stamp every nightly reports the fallback version, and a bug report against one is
// indistinguishable from a bug report against any other.
func TestNightlyStampsTheBuildLabel(t *testing.T) {
	workflow := readNightlyWorkflow(t)

	const symbol = "xquakshell/internal/presentation/wails.AppVersion"
	if !strings.Contains(workflow, "-X "+symbol+"=") {
		t.Errorf("nightly.yml does not pass -ldflags \"-X %s=<label>\"", symbol)
	}
	// The label carries the date and the short SHA, so a downloaded archive can be traced back to
	// the commit it came from even though the tag it hung on has since moved.
	if !strings.Contains(workflow, `label=nightly-$(date -u +%Y%m%d)-$(echo "${SHA}" | cut -c1-7)`) {
		t.Error("the nightly label no longer carries the build date and commit, so an archive " +
			"cannot be traced to the commit that produced it")
	}
}

// A nightly is downloaded over plain HTTPS from a release that is deleted and recreated daily.
// SHA256SUMS is the only way a tester can tell the archive they have is the archive CI built.
func TestNightlyChecksumsEveryPublishedArchive(t *testing.T) {
	publish := jobSection(t, readNightlyWorkflow(t), "publish")

	if !strings.Contains(publish, "sha256sum * > SHA256SUMS") {
		t.Error("the publish job no longer generates SHA256SUMS from the collected archives")
	}
	if !strings.Contains(publish, "dist/SHA256SUMS") {
		t.Error("SHA256SUMS is generated but never attached to the release")
	}
}

// The Windows archives are the ones most testers actually use, and their contents are what the
// release's own tests assert. Comparing the two jobs file-for-file is the check that catches a
// packaging change landing in one workflow and not the other.
func TestNightlyWindowsArchivesMatchTheRelease(t *testing.T) {
	copies := regexp.MustCompile(`Copy-Item "([^"]+)"`)

	want := sourcesCopied(copies, windowsJob(t))
	got := sourcesCopied(copies, jobSection(t, readNightlyWorkflow(t), "windows"))
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the nightly Windows job packages %v, the release job packages %v", got, want)
	}

	// The runtime-carrying archive is worse than useless if the file Wails loads is missing, so
	// the nightly has to stop on it for the same reason the release does.
	job := jobSection(t, readNightlyWorkflow(t), "windows")
	if !strings.Contains(job, "EmbeddedBrowserWebView.dll") || !strings.Contains(job, "throw") {
		t.Error("the nightly Windows job does not fail on a bundled WebView2 runtime that is " +
			"missing the file Wails loads")
	}
}

// Same comparison for Linux, where the archive carries a launcher, a desktop entry and an icon that
// an ELF on its own does not have. Both ABI variants come from one job, so one set covers both.
func TestNightlyLinuxArchivesMatchTheRelease(t *testing.T) {
	copies := regexp.MustCompile(`(?m)^\s*cp (\S+) "stage/\$NAME/`)
	workflow := readNightlyWorkflow(t)

	want := sourcesCopied(copies, jobSection(t, readReleaseWorkflow(t), "linux"))
	if len(want) == 0 {
		t.Fatal("the release Linux job copies nothing into its archive; the pattern no longer matches")
	}
	got := sourcesCopied(copies, jobSection(t, workflow, "linux"))
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the nightly Linux job packages %v, the release job packages %v", got, want)
	}

	// Both ABIs, for the reason release.yml documents: a binary built against 4.0 does not start
	// against 4.1, so a nightly that dropped one would leave half the distributions unbuilt.
	for _, webkit := range []string{"4.0", "4.1"} {
		if !strings.Contains(workflow, "webkit: '"+webkit+"'") {
			t.Errorf("nightly.yml no longer builds the WebKitGTK %s variant", webkit)
		}
	}
}
