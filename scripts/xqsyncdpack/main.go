// Command xqsyncdpack builds the xqsyncd release archives (spec 9.5): windows-amd64 as a zip,
// linux-amd64 and linux-arm64 as tar.gz, each unpacking into one directory named after itself with
// the binary, LICENSE, the admin guide as README.md and, on Linux, the systemd unit.
//
//	go run ./scripts/xqsyncdpack -label v1.5.0 -out dist
//
// release.yml and nightly.yml both call it, so the two cannot package the server differently. The
// server is pure Go (modernc SQLite), so every target cross-compiles from one runner.
package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"time"
)

type target struct{ goos, goarch string }

var targets = []target{{"windows", "amd64"}, {"linux", "amd64"}, {"linux", "arm64"}}

// buildFunc compiles cmd/xqsyncd for one target into out.
type buildFunc func(root string, tg target, out string) error

var safeLabel = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

func main() {
	label := flag.String("label", "", "version label in the archive names, e.g. v1.5.0")
	out := flag.String("out", "dist", "output directory")
	flag.Parse()
	names, err := pack(".", *label, *out, goBuild)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, n := range names {
		fmt.Println(n)
	}
}

func goBuild(root string, tg target, out string) error {
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w", "-o", out, "./cmd/xqsyncd") // #nosec G204 -- the go toolchain with fixed arguments; out is a path this program chose
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS="+tg.goos, "GOARCH="+tg.goarch, "CGO_ENABLED=0")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

// pack builds every target and writes its archive into out, returning the archive names.
func pack(root, label, out string, build buildFunc) ([]string, error) {
	if !safeLabel.MatchString(label) {
		return nil, errors.New("xqsyncdpack: -label must be a plain version label")
	}
	if err := os.MkdirAll(out, 0o750); err != nil {
		return nil, err
	}
	work, err := os.MkdirTemp("", "xqsyncdpack")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(work) }()
	p := packing{root: root, label: label, out: out, work: work, build: build}
	var names []string
	for _, tg := range targets {
		name, err := p.target(tg)
		if err != nil {
			return nil, fmt.Errorf("xqsyncdpack: %s/%s: %w", tg.goos, tg.goarch, err)
		}
		names = append(names, name)
	}
	return names, nil
}

// entry is one file in an archive: where it comes from and whether it is executable.
type entry struct {
	src  string
	exec bool
}

// packing is one run: where the sources are, what the archives are called and where they go.
type packing struct {
	root, label, out, work string
	build                  buildFunc
}

func (p packing) target(tg target) (string, error) {
	root, out := p.root, p.out
	dir := "xqsyncd-" + p.label + "-" + tg.goos + "-" + tg.goarch
	bin := "xqsyncd"
	if tg.goos == "windows" {
		bin += ".exe"
	}
	binPath := filepath.Join(p.work, dir+"-"+bin)
	if err := p.build(root, tg, binPath); err != nil {
		return "", err
	}
	files := map[string]entry{
		bin:         {src: binPath, exec: true},
		"LICENSE":   {src: filepath.Join(root, "LICENSE")},
		"README.md": {src: filepath.Join(root, "docs", "sync-server.md")},
	}
	if tg.goos == "linux" {
		files["xqsyncd.service"] = entry{src: filepath.Join(root, "packaging", "linux", "xqsyncd.service")}
		return dir + ".tar.gz", writeTarGz(filepath.Join(out, dir+".tar.gz"), dir, files)
	}
	return dir + ".zip", writeZip(filepath.Join(out, dir+".zip"), dir, files)
}

// sortedNames gives a fixed order, so two packs of the same inputs differ only in the binaries.
func sortedNames(files map[string]entry) []string { return slices.Sorted(maps.Keys(files)) }

func writeTarGz(path, dir string, files map[string]entry) error {
	f, err := os.Create(path) // #nosec G304 -- the output directory the caller named
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, name := range sortedNames(files) {
		e := files[name]
		body, err := os.ReadFile(e.src)
		if err != nil {
			return errors.Join(err, f.Close())
		}
		mode := int64(0o644)
		if e.exec {
			mode = 0o755
		}
		if err := tw.WriteHeader(&tar.Header{Name: dir + "/" + name, Mode: mode, Size: int64(len(body)), ModTime: time.Now().UTC().Truncate(time.Second)}); err != nil {
			return errors.Join(err, f.Close())
		}
		if _, err := tw.Write(body); err != nil {
			return errors.Join(err, f.Close())
		}
	}
	return errors.Join(tw.Close(), gz.Close(), f.Close())
}

func writeZip(path, dir string, files map[string]entry) error {
	f, err := os.Create(path) // #nosec G304 -- the output directory the caller named
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	for _, name := range sortedNames(files) {
		src, err := os.Open(files[name].src)
		if err != nil {
			return errors.Join(err, f.Close())
		}
		w, err := zw.Create(dir + "/" + name)
		if err == nil {
			_, err = io.Copy(w, src)
		}
		if err := errors.Join(err, src.Close()); err != nil {
			return errors.Join(err, f.Close())
		}
	}
	return errors.Join(zw.Close(), f.Close())
}
