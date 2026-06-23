package main

import (
	"crypto/ed25519"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	domainplugin "ssh-client/internal/domain/plugin"
	infraplugin "ssh-client/internal/infra/plugin"
	"ssh-client/internal/infra/plugin/bundle"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	var err error
	switch cmd {
	case "init":
		err = runInit(args)
	case "build":
		err = runBuild(args)
	case "pack":
		err = runPack(args)
	case "validate":
		err = runValidate(args)
	case "checksums":
		err = runChecksums(args)
	case "sign":
		err = runSign(args)
	case "keygen":
		err = runKeygen(args)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage()
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `xqs-plugin — xQuakShell plugin developer CLI

Usage:
  xqs-plugin init [-id com.example.myplugin] [-name "My Plugin"]
  xqs-plugin build [-dir .] [-o my-plugin.exe]
  xqs-plugin pack [-dir .] [-o my-plugin.xqs-plugin]
  xqs-plugin validate [-path .]
  xqs-plugin checksums [-dir .]
  xqs-plugin sign [-dir .] -key publisher.key
  xqs-plugin keygen [-out publisher.key]

Examples:
  xqs-plugin init -id com.example.echo -name "Example Echo"
  xqs-plugin build
  xqs-plugin pack -o dist/my-plugin.xqs-plugin
  xqs-plugin sign -key publisher.key
`)
}

func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	id := fs.String("id", "com.example.myplugin", "plugin id")
	name := fs.String("name", "My Plugin", "display name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dir := "."
	if fs.NArg() > 0 {
		dir = fs.Arg(0)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	binaryName := slugBase(*id) + exeSuffix()
	manifest := domainplugin.Manifest{
		ID:          *id,
		Name:        *name,
		Version:     "0.1.0",
		Description: "xQuakShell plugin",
		Engine: domainplugin.EngineConfig{
			Type:  domainplugin.EngineGoBinary,
			Entry: binaryName,
		},
		ActivationEvents: []string{"onStartup"},
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), append(data, '\n'), 0644); err != nil {
		return err
	}

	mainGo := `package main

import (
	"encoding/json"
	"log"

	"github.com/xquakshell/pluginsdk"
)

func main() {
	host := pluginsdk.NewHost()
	host.Register("initialize", func(params json.RawMessage) (any, error) {
		log.Printf("plugin initialized")
		return map[string]bool{"ok": true}, nil
	})
	host.Register("ping", func(_ json.RawMessage) (any, error) {
		return map[string]string{"pong": "` + *id + `"}, nil
	})
	if err := host.Serve(); err != nil {
		log.Fatal(err)
	}
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainGo), 0644); err != nil {
		return err
	}

	fmt.Printf("Created plugin scaffold in %s\n", dir)
	fmt.Printf("Next: xqs-plugin build -dir %s\n", dir)
	return nil
}

func runBuild(args []string) error {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	dir := fs.String("dir", ".", "plugin source directory")
	out := fs.String("o", "", "output binary path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	*dir = absPath(*dir)
	if *out == "" {
		*out = defaultBinaryPath(*dir)
	}
	cmd := exec.Command("go", "build", "-ldflags=-s -w", "-trimpath", "-o", *out, ".")
	cmd.Dir = *dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	fmt.Printf("Building %s ...\n", *out)
	return cmd.Run()
}

func runPack(args []string) error {
	fs := flag.NewFlagSet("pack", flag.ExitOnError)
	dir := fs.String("dir", ".", "plugin source directory")
	out := fs.String("o", "", "output .xqs-plugin path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	*dir = absPath(*dir)
	if err := infraplugin.ValidatePluginSource(*dir); err != nil {
		return fmt.Errorf("validate before pack: %w", err)
	}
	if *out == "" {
		base := filepath.Base(*dir)
		if base == "." || base == string(filepath.Separator) {
			base = "plugin"
		}
		*out = base + bundle.Extension
	}
	fmt.Printf("Packing %s -> %s\n", *dir, *out)
	return bundle.Pack(*dir, *out)
}

func runValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	path := fs.String("path", ".", "plugin directory or .xqs-plugin bundle")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := infraplugin.ValidatePluginSource(absPath(*path)); err != nil {
		return err
	}
	fmt.Println("OK")
	return nil
}

func runChecksums(args []string) error {
	fs := flag.NewFlagSet("checksums", flag.ExitOnError)
	dir := fs.String("dir", ".", "plugin directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	*dir = absPath(*dir)
	if err := bundle.WriteChecksums(*dir); err != nil {
		return err
	}
	fmt.Printf("Wrote %s in %s\n", bundle.ChecksumsFile, *dir)
	return nil
}

func runSign(args []string) error {
	fs := flag.NewFlagSet("sign", flag.ExitOnError)
	dir := fs.String("dir", ".", "plugin directory")
	keyPath := fs.String("key", "", "base64 Ed25519 private key file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *keyPath == "" {
		return fmt.Errorf("-key is required")
	}
	*dir = absPath(*dir)
	manifestPath := filepath.Join(*dir, "plugin.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	var manifest domainplugin.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	keyData, err := os.ReadFile(*keyPath)
	if err != nil {
		return err
	}
	priv, err := domainplugin.ParsePrivateKey(strings.TrimSpace(string(keyData)))
	if err != nil {
		return err
	}
	sig, err := domainplugin.SignManifest(manifest, priv)
	if err != nil {
		return err
	}
	manifest.Signature = sig
	out, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(manifestPath, append(out, '\n'), 0644); err != nil {
		return err
	}
	fmt.Printf("Signed %s\n", manifestPath)
	return nil
}

func runKeygen(args []string) error {
	fs := flag.NewFlagSet("keygen", flag.ExitOnError)
	out := fs.String("out", "publisher.key", "output private key file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, []byte(domainplugin.EncodePrivateKey(priv)+"\n"), 0600); err != nil {
		return err
	}
	pubPath := strings.TrimSuffix(*out, filepath.Ext(*out)) + ".pub"
	if err := os.WriteFile(pubPath, []byte(domainplugin.EncodePublicKey(pub)+"\n"), 0644); err != nil {
		return err
	}
	fmt.Printf("Wrote private key %s and public key %s\n", *out, pubPath)
	fmt.Printf("Add the public key to xQuakShell Settings → Plugins → Trusted publishers\n")
	return nil
}

func slugBase(id string) string {
	parts := strings.Split(id, ".")
	if len(parts) == 0 {
		return "plugin"
	}
	return parts[len(parts)-1]
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func absPath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

func defaultBinaryPath(dir string) string {
	manifestPath := filepath.Join(dir, "plugin.json")
	data, err := os.ReadFile(manifestPath)
	if err == nil {
		var manifest domainplugin.Manifest
		if json.Unmarshal(data, &manifest) == nil && manifest.Engine.Entry != "" {
			return filepath.Join(dir, manifest.Engine.Entry)
		}
	}
	return filepath.Join(dir, "plugin"+exeSuffix())
}
