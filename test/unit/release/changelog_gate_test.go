package release_test

import (
	"strings"
	"testing"
)

// The changelog gate lives in the workflow because only a tagged run knows which version is being
// published. A step in a YAML file has nothing else pointing at it, so deleting it is a one-line
// change that no build would notice — which is how a gate quietly stops existing. These assertions
// are the thing that notices.

func publishJob(t *testing.T) string {
	t.Helper()
	workflow := readReleaseWorkflow(t)
	start := strings.Index(workflow, "\n  publish:")
	if start < 0 {
		t.Fatal("release.yml no longer declares a publish job; this gate has stopped checking anything")
	}
	return workflow[start:]
}

func TestPublishVerifiesTheChangelogBeforeCreatingTheRelease(t *testing.T) {
	job := publishJob(t)

	verify := strings.Index(job, "Verify the changelog covers this tag")
	if verify < 0 {
		t.Fatal("the publish job no longer verifies the changelog; a tag could be published with " +
			"no record of what changed, and a tag cannot be taken back")
	}

	create := strings.Index(job, "Create GitHub Release")
	if create < 0 {
		t.Fatal("the publish job no longer creates a release")
	}
	if verify > create {
		t.Error("the changelog is verified after the release is created; by then the release is " +
			"already public and the check cannot prevent anything")
	}
}

// The step reads CHANGELOG.md and wails.json out of the working directory. The publish job used to
// download artifacts and nothing else, so without a checkout those files are simply absent and the
// gate would fail for the wrong reason on every release.
func TestPublishChecksOutTheRepository(t *testing.T) {
	if !strings.Contains(publishJob(t), "actions/checkout") {
		t.Error("the publish job does not check out the repository; the changelog gate has no " +
			"CHANGELOG.md to read")
	}
}

// Each of these is a distinct way to publish a release nobody can act on: a missing section, an
// entry still marked as being prepared, a version that disagrees with the binary's own resource
// block, or an entry with no compatibility or breaking-change record.
func TestChangelogGateCoversEveryIncompleteEntry(t *testing.T) {
	job := publishJob(t)
	for _, needle := range []string{
		`## \[$version\]`,
		"nreleased",
		"### Compatibility",
		"### BREAKING",
		"productVersion",
	} {
		if !strings.Contains(job, needle) {
			t.Errorf("the changelog gate no longer checks %q", needle)
		}
	}
}
