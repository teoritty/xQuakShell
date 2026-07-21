package architecture_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"xquakshell/test/unit/architecture"
)

func TestLayerImportRulesMeta(t *testing.T) {
	const module = "xquakshell"

	cases := []struct {
		name           string
		layer          architecture.Layer
		importPath     string
		productionFile bool
		wantViolation  bool
	}{
		// domain
		{name: "domain allows internal/domain", layer: architecture.LayerDomain, importPath: "xquakshell/internal/domain", productionFile: true, wantViolation: false},
		{name: "domain allows golang.org/x/crypto/ssh", layer: architecture.LayerDomain, importPath: "golang.org/x/crypto/ssh", productionFile: true, wantViolation: false},
		{name: "domain allows stdlib", layer: architecture.LayerDomain, importPath: "context", productionFile: true, wantViolation: false},
		{name: "domain forbids infra", layer: architecture.LayerDomain, importPath: "xquakshell/internal/infra/vault", productionFile: true, wantViolation: true},
		{name: "domain forbids golang.org/x/sys", layer: architecture.LayerDomain, importPath: "golang.org/x/sys/windows", productionFile: true, wantViolation: true},
		{name: "domain forbids github.com", layer: architecture.LayerDomain, importPath: "github.com/google/uuid", productionFile: true, wantViolation: true},

		// usecase production
		{name: "usecase allows domain", layer: architecture.LayerUsecase, importPath: "xquakshell/internal/domain", productionFile: true, wantViolation: false},
		{name: "usecase allows safego", layer: architecture.LayerUsecase, importPath: "xquakshell/internal/pkg/safego", productionFile: true, wantViolation: false},
		{name: "usecase forbids infra", layer: architecture.LayerUsecase, importPath: "xquakshell/internal/infra/vault", productionFile: true, wantViolation: true},
		{name: "usecase forbids presentation", layer: architecture.LayerUsecase, importPath: "xquakshell/internal/presentation/wails", productionFile: true, wantViolation: true},
		{name: "usecase forbids other pkg", layer: architecture.LayerUsecase, importPath: "xquakshell/internal/pkg/conlimit", productionFile: true, wantViolation: true},
		{name: "usecase forbids golang.org/x", layer: architecture.LayerUsecase, importPath: "golang.org/x/crypto/ssh", productionFile: true, wantViolation: true},
		{name: "usecase forbids github.com", layer: architecture.LayerUsecase, importPath: "github.com/google/uuid", productionFile: true, wantViolation: true},
		{name: "usecase test allows golang.org/x", layer: architecture.LayerUsecase, importPath: "golang.org/x/crypto/ssh", productionFile: false, wantViolation: false},

		// infra
		{name: "infra allows domain", layer: architecture.LayerInfra, importPath: "xquakshell/internal/domain", productionFile: true, wantViolation: false},
		{name: "infra allows pkg", layer: architecture.LayerInfra, importPath: "xquakshell/internal/pkg/pathsafe", productionFile: true, wantViolation: false},
		{name: "infra allows third-party", layer: architecture.LayerInfra, importPath: "github.com/pkg/sftp", productionFile: true, wantViolation: false},
		{name: "infra forbids usecase", layer: architecture.LayerInfra, importPath: "xquakshell/internal/usecase", productionFile: true, wantViolation: true},
		{name: "infra forbids presentation", layer: architecture.LayerInfra, importPath: "xquakshell/internal/presentation/wails", productionFile: true, wantViolation: true},

		// presentation
		{name: "presentation allows usecase", layer: architecture.LayerPresentation, importPath: "xquakshell/internal/usecase", productionFile: true, wantViolation: false},
		{name: "presentation allows wails", layer: architecture.LayerPresentation, importPath: "github.com/wailsapp/wails/v2/pkg/runtime", productionFile: true, wantViolation: false},
		{name: "presentation allows x/sys", layer: architecture.LayerPresentation, importPath: "golang.org/x/sys/windows", productionFile: true, wantViolation: false},
		{name: "presentation forbids infra", layer: architecture.LayerPresentation, importPath: "xquakshell/internal/infra/vault", productionFile: true, wantViolation: true},
		{name: "presentation forbids other pkg", layer: architecture.LayerPresentation, importPath: "xquakshell/internal/pkg/conlimit", productionFile: true, wantViolation: true},
		{name: "presentation forbids other third-party", layer: architecture.LayerPresentation, importPath: "github.com/google/uuid", productionFile: true, wantViolation: true},

		// main
		{name: "main allows anything internal", layer: architecture.LayerMain, importPath: "xquakshell/internal/infra/vault", productionFile: true, wantViolation: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := architecture.CheckImportForLayer(module, tc.importPath, tc.layer, tc.productionFile)
			gotViolation := v != nil
			if gotViolation != tc.wantViolation {
				if tc.wantViolation {
					t.Fatalf("expected violation for %q in layer %s", tc.importPath, tc.layer)
				}
				t.Fatalf("unexpected violation for %q in layer %s: %+v", tc.importPath, tc.layer, v)
			}
		})
	}
}

func TestViolationFixtures(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	fixtureDir := filepath.Join(repoRoot, "test", "unit", "architecture", "testdata", "violations")
	entries, err := os.ReadDir(fixtureDir)
	if err != nil {
		t.Fatal(err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		path := filepath.Join(fixtureDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		src := string(data)
		layerLine := ""
		for _, line := range strings.Split(src, "\n") {
			if strings.HasPrefix(line, "// layer:") {
				layerLine = strings.TrimSpace(strings.TrimPrefix(line, "// layer:"))
				break
			}
		}
		if layerLine == "" {
			t.Fatalf("%s: missing // layer: directive", entry.Name())
		}
		layer := architecture.Layer(layerLine)
		imports, err := architecture.ParseImportsFromSource(src)
		if err != nil {
			t.Fatalf("%s: parse imports: %v", entry.Name(), err)
		}
		if len(imports) == 0 {
			t.Fatalf("%s: expected at least one import", entry.Name())
		}
		for _, imp := range imports {
			if imp == "context" || imp == "testing" {
				continue
			}
			v := architecture.CheckImportForLayer("xquakshell", imp, layer, true)
			if v == nil {
				t.Fatalf("%s: expected violation for import %q in layer %s", entry.Name(), imp, layer)
			}
		}
	}
}
