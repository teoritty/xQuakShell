package plugin_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

// The manifest schema is a published contract with no version number on it: a plugin author writes
// plugin.json against whatever this build accepts, and a manifest that was valid must stay valid.
// Nothing about that is visible in a diff — tightening one validation branch, or adding a field
// without a default, silently stops every published plugin from installing.
//
// So the golden records two different things, because neither alone is the contract. The shape
// says which fields exist and how they serialise; the behaviour says which of them Validate
// actually insists on. A field can be added to the struct without changing what is required, and a
// requirement can be tightened without touching the struct, and only one of those shows up in each
// half.
//
// Regenerate with: go test ./internal/domain/plugin -run TestManifestSchema -update-golden
// The -update-golden flag is declared once in api_contract_test.go; redeclaring it here would
// panic the package at init with "flag redefined".

const manifestGoldenPath = "testdata/manifest_schema.golden.json"

type schemaField struct {
	Name      string `json:"name"`
	JSONName  string `json:"json"`
	OmitEmpty bool   `json:"omitempty"`
	Type      string `json:"type"`
}

type schemaType struct {
	Name   string        `json:"name"`
	Fields []schemaField `json:"fields"`
}

type schemaRequirement struct {
	Path     string `json:"path"`
	Required bool   `json:"required"`
}

type manifestSchema struct {
	Types        []schemaType        `json:"types"`
	Requirements []schemaRequirement `json:"requirements"`
}

// structTypesFrom collects every struct type reachable from root through fields, slices, maps and
// pointers, so a change buried in a contribution type is recorded too.
func structTypesFrom(root reflect.Type) map[string]reflect.Type {
	found := map[string]reflect.Type{}
	var visit func(reflect.Type)
	visit = func(t reflect.Type) {
		for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Map {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct || t.PkgPath() == "" {
			return
		}
		if _, seen := found[t.Name()]; seen {
			return
		}
		found[t.Name()] = t
		for i := 0; i < t.NumField(); i++ {
			visit(t.Field(i).Type)
		}
	}
	visit(root)
	return found
}

func schemaShape(root reflect.Type) []schemaType {
	types := structTypesFrom(root)
	names := make([]string, 0, len(types))
	for name := range types {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]schemaType, 0, len(names))
	for _, name := range names {
		t := types[name]
		entry := schemaType{Name: name}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" {
				continue // unexported: not part of the wire contract
			}
			tag := strings.Split(f.Tag.Get("json"), ",")
			jsonName := f.Name
			if tag[0] != "" {
				jsonName = tag[0]
			}
			entry.Fields = append(entry.Fields, schemaField{
				Name:      f.Name,
				JSONName:  jsonName,
				OmitEmpty: strings.Contains(f.Tag.Get("json"), ",omitempty"),
				Type:      f.Type.String(),
			})
		}
		out = append(out, entry)
	}
	return out
}

// minimalValidManifest is the smallest manifest this build accepts. Clearing one field of it at a
// time is what turns "required" from a claim in a doc comment into something a diff can show.
func minimalValidManifest() domainplugin.Manifest {
	return domainplugin.Manifest{
		ID:      "com.example.echo",
		Name:    "Echo",
		Version: "1.0.0",
		Engine: domainplugin.EngineConfig{
			Type:  domainplugin.EngineGoBinary,
			Entry: "echo.exe",
		},
	}
}

// clearablePaths lists every exported field of the manifest, descending one level into plain
// struct fields so engine.type and engine.entry are covered individually.
func clearablePaths(t reflect.Type, prefix string, index []int) []struct {
	path  string
	index []int
} {
	type entry = struct {
		path  string
		index []int
	}
	var out []entry
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue
		}
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if name == "" {
			name = f.Name
		}
		here := append(append([]int{}, index...), i)
		out = append(out, entry{path: prefix + name, index: here})
		if f.Type.Kind() == reflect.Struct && f.Type.PkgPath() != "" && prefix == "" {
			out = append(out, clearablePaths(f.Type, prefix+name+".", here)...)
		}
	}
	return out
}

func schemaRequirements(t *testing.T) []schemaRequirement {
	t.Helper()
	base := minimalValidManifest()
	if err := base.Validate(); err != nil {
		t.Fatalf("the smallest manifest this build used to accept is now rejected: %v\n"+
			"Validate has gained a requirement on a field that was optional, which stops every "+
			"published manifest omitting it from installing. That needs a major bump and a "+
			"deprecation window (ADR-017), not a new branch in Validate.", err)
	}

	var out []schemaRequirement
	for _, p := range clearablePaths(reflect.TypeFor[domainplugin.Manifest](), "", nil) {
		m := minimalValidManifest()
		field := reflect.ValueOf(&m).Elem().FieldByIndex(p.index)
		field.Set(reflect.Zero(field.Type()))
		out = append(out, schemaRequirement{Path: p.path, Required: m.Validate() != nil})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func TestManifestSchemaIsFrozen(t *testing.T) {
	schema := manifestSchema{
		Types:        schemaShape(reflect.TypeFor[domainplugin.Manifest]()),
		Requirements: schemaRequirements(t),
	}
	got, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')

	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(manifestGoldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(manifestGoldenPath, got, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote golden %s", manifestGoldenPath)
		return
	}

	want, err := os.ReadFile(manifestGoldenPath)
	if err != nil {
		t.Fatalf("read golden (run with -update-golden to create): %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("the plugin.json schema changed.\n--- got ---\n%s\n--- want ---\n%s\nIf intentional, regenerate with -update-golden and review the diff against ADR-017.", got, want)
	}
}

// The additive-only half. A golden diff shows that something moved; this says which direction is
// allowed to move. Removing a field or making an optional one required breaks manifests already
// published against this schema, and both need a major bump on pluginApi plus a deprecation window.
func TestManifestSchemaChangesAreAdditiveOnly(t *testing.T) {
	data, err := os.ReadFile(manifestGoldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var golden manifestSchema
	if err := json.Unmarshal(data, &golden); err != nil {
		t.Fatal(err)
	}

	current := map[string]map[string]schemaField{}
	for _, typ := range schemaShape(reflect.TypeFor[domainplugin.Manifest]()) {
		fields := map[string]schemaField{}
		for _, f := range typ.Fields {
			fields[f.JSONName] = f
		}
		current[typ.Name] = fields
	}

	for _, typ := range golden.Types {
		fields, ok := current[typ.Name]
		if !ok {
			t.Errorf("type %q was removed from the manifest schema", typ.Name)
			continue
		}
		for _, f := range typ.Fields {
			if _, ok := fields[f.JSONName]; !ok {
				t.Errorf("%s.%s was removed; a manifest that declares it stops parsing as intended", typ.Name, f.JSONName)
			}
		}
	}

	required := map[string]bool{}
	for _, r := range schemaRequirements(t) {
		required[r.Path] = r.Required
	}
	for _, r := range golden.Requirements {
		if !r.Required && required[r.Path] {
			t.Errorf("%q became required; every published manifest that omits it stops installing", r.Path)
		}
	}
}
