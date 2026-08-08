package docs_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The link gate is a set of glob patterns in a YAML string. Nothing about a green run says those
// globs matched anything: edit one into a typo and lychee reports zero links, finds zero errors and
// passes. This repository already carries a scar from exactly that shape - file_size.go says it in
// as many words, "a budget that covers nothing is an error", after a PowerShell gate matched no
// files for years.
//
// So the globs are checked against the tree rather than trusted: every markdown file that is not in
// a generated or vendored directory must be covered by one of them. A new document in a new place
// fails here, which is the moment to either widen the glob or say why it is exempt.

// Directories whose markdown is not ours to check: dependencies, build output, and the sandbox
// Stryker writes during a mutation run.
var uncheckedTrees = map[string]bool{
	"node_modules":    true,
	"vendor":          true,
	"build":           true,
	"dist":            true,
	".git":            true,
	".stryker-tmp":    true,
	"wailsjs":         true,
	"coverage":        true,
	"reports":         true,
	".repowise":       true,
	".claude":         true,
	".superpowers":    true,
	".github-private": true,
}

func TestLinkCheckCoversEveryDocument(t *testing.T) {
	root := repoRoot(t)
	patterns := lycheePatterns(t, readRepoFile(t, ".github", "workflows", "test.yml"))
	if len(patterns) == 0 {
		t.Fatal("no glob patterns found in the lychee step; the link gate would check nothing")
	}

	var uncovered []string
	found := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if uncheckedTrees[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		found++
		slashed := filepath.ToSlash(rel)
		if !coveredBy(patterns, slashed) {
			uncovered = append(uncovered, slashed)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// A walk that finds nothing would make every assertion below vacuous, which is the failure this
	// whole file exists to prevent - so it is an error here too, not a quiet pass.
	if found == 0 {
		t.Fatal("no markdown files found under the repository root; this check is checking nothing")
	}
	for _, doc := range uncovered {
		t.Errorf("%s is matched by no glob in the lychee step, so its links are never checked; "+
			"widen the args in test.yml or exclude the directory deliberately", doc)
	}
}

// lycheePatterns pulls the quoted globs out of the step's args line.
func lycheePatterns(t *testing.T, workflow string) []string {
	t.Helper()
	step := regexp.MustCompile(`(?m)^\s*args: (--offline.*)$`).FindStringSubmatch(workflow)
	if step == nil {
		t.Fatal("test.yml has no offline lychee args line; the link gate has moved or gone")
	}
	return regexp.MustCompile(`'([^']+)'`).FindAllString(step[1], -1)
}

func coveredBy(patterns []string, rel string) bool {
	for _, quoted := range patterns {
		if globMatches(strings.Trim(quoted, "'"), rel) {
			return true
		}
	}
	return false
}

// globMatches implements the two wildcards the patterns actually use. filepath.Match cannot: it has
// no "**", and it refuses to let "*" span a separator, which is the whole difference here.
func globMatches(pattern, rel string) bool {
	var expr strings.Builder
	expr.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch {
		case strings.HasPrefix(pattern[i:], "**/"):
			expr.WriteString(`(?:[^/]+/)*`) // any number of directories, including none
			i += 2
		case pattern[i] == '*':
			expr.WriteString(`[^/]*`)
		default:
			expr.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	expr.WriteString("$")
	return regexp.MustCompile(expr.String()).MatchString(rel)
}
