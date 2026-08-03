package release_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The Linux archive is portable: it is unpacked wherever the user likes and run from there. That
// makes the archive itself the contract — everything the app needs to start, and to register a
// launcher, has to be inside it, and the names inside it have to agree with each other. None of
// that is checked by the compiler, and a broken archive only shows up on a user's machine, so the
// assertions live here.
//
// The other half of the contract is the WebKitGTK ABI. Wails links the system WebKitGTK, which
// cannot be bundled: distributions ship either the old webkit2gtk-4.0 or the current 4.1, and a
// binary linked against one does not start against the other. One archive therefore cannot serve
// every supported distribution, and the release publishes one per ABI.

func readPackagingFile(t *testing.T, name string) string {
	t.Helper()
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "packaging", "linux", name))
	if err != nil {
		t.Fatalf("packaging/linux/%s is shipped in the archive but missing from the repo: %v", name, err)
	}
	return string(raw)
}

// linuxJob returns the portion of release.yml belonging to the linux job, so an assertion about
// the Linux build cannot be satisfied by an unrelated line in the Windows job.
func linuxJob(t *testing.T) string {
	t.Helper()
	workflow := readReleaseWorkflow(t)
	start := strings.Index(workflow, "\n  linux:")
	if start < 0 {
		t.Fatal("release.yml has no linux job; the release would publish Windows archives only")
	}
	rest := workflow[start+1:]
	// Jobs are two-space indented; the next such key ends this one.
	if end := regexp.MustCompile(`\n  [a-z][\w-]*:`).FindStringIndex(rest); end != nil {
		return rest[:end[0]]
	}
	return rest
}

// A binary linked against webkit2gtk-4.0 fails to start on Ubuntu 24.04+ or Fedora 40+, and a 4.1
// binary fails to start on the older systems that only ship 4.0. Both have to be built, and the
// build tag is what selects the ABI — a 4.1 build without it silently links 4.0.
func TestLinuxBuildsCoverBothWebKitABIs(t *testing.T) {
	job := linuxJob(t)

	for _, dev := range []string{"libwebkit2gtk-4.0-dev", "libwebkit2gtk-4.1-dev"} {
		if !strings.Contains(job, dev) {
			t.Errorf("the linux job never installs %s; one of the two WebKitGTK ABIs is not built", dev)
		}
	}
	if !strings.Contains(job, "webkit2_41") {
		t.Error("the linux job never passes -tags webkit2_41; the 4.1 archive would link 4.0 and " +
			"fail to start on every distribution that dropped it")
	}
}

// Building the 4.1 variant on a newer runner would raise the glibc floor of both archives and lock
// out the distributions the 4.0 archive exists to serve. Both variants build on the oldest runner
// that still carries a 4.0 development package.
func TestLinuxBuildsOnTheOldestSupportedRunner(t *testing.T) {
	job := linuxJob(t)
	if !strings.Contains(job, "runs-on: ubuntu-22.04") {
		t.Error("the linux job does not run on ubuntu-22.04; a newer runner raises the glibc floor " +
			"of the published binaries and drops support for older distributions")
	}
}

// Nothing else publishes the Linux archives: if the publish job does not wait for the Linux build,
// a release goes out with Windows archives only and a SHA256SUMS that silently omits Linux.
func TestPublishWaitsForTheLinuxBuild(t *testing.T) {
	workflow := readReleaseWorkflow(t)
	needs := regexp.MustCompile(`needs:\s*\[([^\]]*)\]`).FindStringSubmatch(workflow)
	if needs == nil {
		t.Fatal("the publish job declares no needs; it could run before any build finishes")
	}
	if !strings.Contains(needs[1], "linux") {
		t.Errorf("publish needs [%s]; the linux job is missing, so its archives would not be published", needs[1])
	}
}

// The archive is the whole product on Linux: an ELF alone has no icon, no launcher and no way to
// appear in an application menu. Every file the desktop entry and the launcher depend on has to be
// copied into the staging directory, and each one has to exist in the repo.
func TestLinuxArchiveShipsEverythingItNeeds(t *testing.T) {
	job := linuxJob(t)

	for _, path := range []string{
		"packaging/linux/xquakshell.sh",
		"packaging/linux/install-desktop-entry.sh",
		"packaging/linux/xquakshell.desktop",
		"packaging/linux/README.md",
		"images/icon.png",
		"LICENSE",
	} {
		if !strings.Contains(job, path) {
			t.Errorf("the linux job never copies %s into the archive", path)
		}
		root, err := findRepoRoot()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			t.Errorf("%s is copied into the archive but does not exist: %v", path, err)
		}
	}
}

// core.filemode is false in this repository and the scripts are tracked as 0644, so the executable
// bit does not survive a checkout. tar records the mode it finds, so without an explicit chmod the
// user unpacks files they cannot run.
func TestLinuxArchiveMakesItsScriptsExecutable(t *testing.T) {
	job := linuxJob(t)
	if !strings.Contains(job, "chmod +x") {
		t.Error("the linux job never runs chmod +x; the executable bit is not carried by the " +
			"repository (core.filemode=false, scripts tracked 0644), so nothing in the archive would run")
	}
}

// A tarball that unpacks its contents into the current directory scatters files over whatever the
// user happened to be in. Every archive unpacks into one directory named after itself.
func TestLinuxArchiveUnpacksIntoItsOwnDirectory(t *testing.T) {
	job := linuxJob(t)
	if !strings.Contains(job, "tar ") {
		t.Fatal("the linux job produces no tarball")
	}
	if !regexp.MustCompile(`tar[^\n]*-C `).MatchString(job) {
		t.Error("tar is invoked without -C <staging dir>; the archive would either carry the " +
			"build path as its top-level directory or unpack straight into the current directory")
	}
}

// The ABI is not visible from a file name like "linux-amd64", and picking the wrong archive fails
// at startup with a missing-shared-library error rather than anything actionable. The name has to
// say which one it is.
func TestLinuxArchiveNamesCarryTheABI(t *testing.T) {
	job := linuxJob(t)
	if !regexp.MustCompile(`xQuakShell-[^\s"]*linux-amd64-webkit`).MatchString(job) {
		t.Error("the archive name does not follow xQuakShell-<tag>-linux-amd64-webkit<abi>; " +
			"a user cannot tell the two Linux archives apart")
	}
	if !strings.Contains(job, ".tar.gz") {
		t.Error("the Linux archive is not a .tar.gz; the publish job's collect step only moves " +
			"*.zip and *.tar.gz, so it would not be published")
	}
}

// The desktop entry points at files by name. Those names are only correct if the files it names are
// the ones actually shipped — a rename on either side leaves a launcher that starts nothing.
func TestDesktopEntryPointsAtShippedFiles(t *testing.T) {
	entries := readDesktopEntry(t)
	job := linuxJob(t)

	for key, want := range map[string]string{"Exec": "xquakshell.sh", "Icon": "icon.png"} {
		value := entries[key]
		if !strings.HasSuffix(value, want) {
			t.Errorf("%s = %q, but the archive ships %q", key, value, want)
		}
		if !strings.Contains(job, want) {
			t.Errorf("the desktop entry's %s names %q, which the linux job never puts in the archive", key, want)
		}
	}
}

// The launcher runs before the app does, on a machine whose shell is unknown. /bin/sh is the only
// interpreter guaranteed to be there.
func TestLinuxScriptsArePosixShell(t *testing.T) {
	for _, name := range []string{"xquakshell.sh", "install-desktop-entry.sh"} {
		body := readPackagingFile(t, name)
		if !strings.HasPrefix(body, "#!/bin/sh\n") {
			t.Errorf("%s does not start with #!/bin/sh; it may not run on a system without bash", name)
		}
		if strings.Contains(body, "\r\n") {
			t.Errorf("%s has CRLF line endings; the kernel would read the shebang as \"/bin/sh\\r\" "+
				"and refuse to execute it", name)
		}
	}
}

// The app resolves its portable data directory from os.Executable(), so the launcher must exec the
// real binary rather than a copy or a shell function — anything that changes the executable's path
// moves the vault, the audit log and every installed plugin somewhere else.
func TestLauncherExecsTheBinaryNextToIt(t *testing.T) {
	body := readPackagingFile(t, "xquakshell.sh")
	if !strings.Contains(body, "readlink -f") {
		t.Error("the launcher does not resolve its own path with readlink -f; started through a " +
			"symlink or a desktop entry it would not find the binary beside it")
	}
	// Either spelling is fine — exec "$here/xQuakShell" directly, or via a variable holding it —
	// as long as the path is built from the resolved directory and the process is handed over.
	if !regexp.MustCompile(`"\$[A-Za-z_]+/xQuakShell"`).MatchString(body) {
		t.Error("the launcher does not build the binary path from its own resolved directory; " +
			"run from elsewhere it would start the wrong binary or none at all")
	}
	if !regexp.MustCompile(`\bexec\s+"\$`).MatchString(body) {
		t.Error("the launcher does not exec the binary; running it as a child process leaves a " +
			"shell in between, so the desktop entry tracks the wrapper rather than the app")
	}
}

// The substitution the desktop entry is built around: the install step is the only thing that knows
// where the archive was unpacked, and a leftover placeholder produces an entry that launches nothing.
func TestInstallScriptSubstitutesThePlaceholder(t *testing.T) {
	body := readPackagingFile(t, "install-desktop-entry.sh")

	if !strings.Contains(body, "%%INSTALL_DIR%%") {
		t.Error("the install script never substitutes %%INSTALL_DIR%%; the installed entry would " +
			"keep the placeholder and launch nothing")
	}
	if !strings.Contains(body, "applications") {
		t.Error("the install script does not write into an applications directory, so no desktop " +
			"environment would pick the entry up")
	}
	if !strings.Contains(body, "XDG_DATA_HOME") {
		t.Error("the install script ignores XDG_DATA_HOME and assumes ~/.local/share; " +
			"the entry lands outside the search path on a system that sets it")
	}
}
