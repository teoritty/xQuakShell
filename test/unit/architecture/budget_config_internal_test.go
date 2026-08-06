package architecture

import (
	"path/filepath"
	"strings"
	"testing"
)

// goodReason is long enough to clear minExemptionReason so the other rules can
// be tested in isolation.
const goodReason = "this file is a declarations-only port surface with no branching at all"

func validConfig() BudgetConfig {
	return BudgetConfig{
		Limits: Limits{
			Go:     FileLimit{MaxCodeLines: 350},
			TS:     FileLimit{MaxCodeLines: 350},
			Svelte: SvelteLimit{MaxScriptCodeLines: 200, MaxTotalCodeLines: 400},
			GoFunc: FuncLimit{MaxCodeLines: 80, MaxParams: 5, MaxNesting: 3},
			TSFunc: FuncLimit{MaxCodeLines: 80, MaxParams: 5, MaxNesting: 3},
		},
		Baseline: Baseline{
			Files:     map[string]FileMeasurement{},
			Functions: map[string]FuncMeasurement{},
		},
	}
}

func TestBudgetConfigValidateRejects(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*BudgetConfig)
		wantErr string
	}{
		{
			name:    "a limit of zero would silently disable the gate",
			mutate:  func(c *BudgetConfig) { c.Limits.Go.MaxCodeLines = 0 },
			wantErr: "limits.go.maxCodeLines must be a positive number",
		},
		{
			name: "an exemption without a real reason",
			mutate: func(c *BudgetConfig) {
				c.Exemptions.Files = []FileExemption{{Path: "a.go", Kind: "port-surface", Reason: "too big"}}
			},
			wantErr: "exemption reason must be at least",
		},
		{
			name: "an invented exemption kind",
			mutate: func(c *BudgetConfig) {
				c.Exemptions.Files = []FileExemption{{Path: "a.go", Kind: "misc", Reason: goodReason}}
			},
			wantErr: `unknown exemption kind "misc"`,
		},
		{
			name: "debt dressed up as an exemption",
			mutate: func(c *BudgetConfig) {
				c.Exemptions.Files = []FileExemption{{Path: "a.go", Kind: "debt", Reason: goodReason}}
			},
			wantErr: "Debt belongs in baseline",
		},
		{
			name: "the same file exempted twice",
			mutate: func(c *BudgetConfig) {
				c.Exemptions.Files = []FileExemption{
					{Path: "a.go", Kind: "port-surface", Reason: goodReason},
					{Path: "a.go", Kind: "generated", Reason: goodReason},
				}
			},
			wantErr: "duplicate file exemption",
		},
		{
			name: "a file that is both exempted and baselined",
			mutate: func(c *BudgetConfig) {
				c.Exemptions.Files = []FileExemption{{Path: "a.go", Kind: "port-surface", Reason: goodReason}}
				c.Baseline.Files["a.go"] = FileMeasurement{CodeLines: 400}
			},
			wantErr: "both exempted and baselined",
		},
		{
			name: "a function exemption with an invented kind",
			mutate: func(c *BudgetConfig) {
				c.Exemptions.Functions = []FuncExemption{{Symbol: "a.go::F", Kind: "port-surface", Reason: goodReason}}
			},
			wantErr: `unknown exemption kind "port-surface"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig()
			tc.mutate(&cfg)
			err := cfg.validate()
			if err == nil {
				t.Fatalf("expected an error containing %q, got none", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestBudgetConfigValidateAccepts(t *testing.T) {
	cfg := validConfig()
	cfg.Exemptions.Files = []FileExemption{{Path: "app.go", Kind: "facade-surface", Reason: goodReason}}
	cfg.Exemptions.Functions = []FuncExemption{{Symbol: "main_compose.go::composeApp", Kind: "composition-root", Reason: goodReason}}
	cfg.Baseline.Files["big.go"] = FileMeasurement{CodeLines: 400}

	if err := cfg.validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

// The shipped config has to satisfy its own rules, and a limit accidentally
// dropped from the JSON must not read as zero.
func TestShippedBudgetConfigLoads(t *testing.T) {
	cfg, err := LoadBudgetConfig(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Limits.Go.MaxCodeLines <= 0 || cfg.Limits.Svelte.MaxScriptCodeLines <= 0 {
		t.Fatalf("limits did not load: %+v", cfg.Limits)
	}
}
