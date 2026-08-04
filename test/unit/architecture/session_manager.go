package architecture

import (
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"strings"
)

// SessionManagerIssue is a violation of the SessionManager decomposition
// (ADR-009). If these fire, logic is moving back into the God Object.
type SessionManagerIssue struct {
	File string
	Line int
	Rule string
}

func (i SessionManagerIssue) String() string {
	if i.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", i.File, i.Line, i.Rule)
	}
	return fmt.Sprintf("%s: %s", i.File, i.Rule)
}

const (
	sessionManagerFile    = "internal/usecase/session_manager.go"
	sessionIOServiceFile  = "internal/usecase/session_io_service.go"
	sessionRegistryFile   = "session_registry.go"
	sessionManagerMaxLine = 120
)

// sessionStateAccessors are the receiver expressions that only the registry
// may touch: everyone else goes through its API.
var sessionStateAccessors = map[string]bool{
	"m.mu.Lock":  true,
	"m.mu.RLock": true,
}

// CheckSessionManagerBoundaries enforces ADR-009.
func CheckSessionManagerBoundaries(repoRoot string) ([]SessionManagerIssue, error) {
	var issues []SessionManagerIssue

	facadeIssues, err := checkSessionManagerFacadeSize(repoRoot)
	if err != nil {
		return nil, err
	}
	issues = append(issues, facadeIssues...)

	stateIssues, err := checkSessionStateEncapsulation(repoRoot)
	if err != nil {
		return nil, err
	}
	issues = append(issues, stateIssues...)

	ioIssues, err := checkInitSessionIOWaits(repoRoot)
	if err != nil {
		return nil, err
	}
	return append(issues, ioIssues...), nil
}

// 1. The facade stays a thin delegate.
func checkSessionManagerFacadeSize(repoRoot string) ([]SessionManagerIssue, error) {
	absPath := filepath.Join(repoRoot, filepath.FromSlash(sessionManagerFile))
	if _, err := os.Stat(absPath); err != nil {
		return []SessionManagerIssue{{
			File: sessionManagerFile,
			Rule: "expected facade file is missing",
		}}, nil
	}

	count, err := countNonBlankLines(absPath)
	if err != nil {
		return nil, err
	}
	if count > sessionManagerMaxLine {
		return []SessionManagerIssue{{
			File: sessionManagerFile,
			Rule: fmt.Sprintf("%d lines exceeds the delegate-only budget of %d (ADR-009)", count, sessionManagerMaxLine),
		}}, nil
	}
	return nil, nil
}

// 2. Only session_registry.go touches the sessions map and its mutex.
func checkSessionStateEncapsulation(repoRoot string) ([]SessionManagerIssue, error) {
	dir := filepath.Join(repoRoot, "internal", "usecase")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read internal/usecase: %w", err)
	}

	var issues []SessionManagerIssue
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "session_") || !strings.HasSuffix(name, ".go") {
			continue
		}
		if strings.HasSuffix(name, "_test.go") || name == sessionRegistryFile {
			continue
		}

		parsed, err := parseGoFile(repoRoot, filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}

		ast.Inspect(parsed.File, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.IndexExpr:
				if selectorPath(node.X) != "m.sessions" {
					return true
				}
				issues = append(issues, SessionManagerIssue{
					File: parsed.RelPath,
					Line: parsed.Fset.Position(node.Pos()).Line,
					Rule: "direct access to the sessions map is confined to session_registry.go (ADR-009)",
				})
			case *ast.CallExpr:
				path := selectorPath(node.Fun)
				if !sessionStateAccessors[path] {
					return true
				}
				issues = append(issues, SessionManagerIssue{
					File: parsed.RelPath,
					Line: parsed.Fset.Position(node.Pos()).Line,
					Rule: fmt.Sprintf("direct %s is confined to session_registry.go (ADR-009)", path),
				})
			}
			return true
		})
	}
	return issues, nil
}

// 3. InitSessionIO blocks on the registry instead of busy-polling.
func checkInitSessionIOWaits(repoRoot string) ([]SessionManagerIssue, error) {
	absPath := filepath.Join(repoRoot, filepath.FromSlash(sessionIOServiceFile))
	parsed, err := parseGoFile(repoRoot, absPath)
	if err != nil {
		return nil, err
	}

	fn := findMethod(parsed.File, "SessionIOService", "InitSessionIO")
	if fn == nil || fn.Body == nil {
		return []SessionManagerIssue{{
			File: sessionIOServiceFile,
			Rule: "expected method (*SessionIOService).InitSessionIO is missing",
		}}, nil
	}

	var issues []SessionManagerIssue
	waitsOnRegistry := false

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		path := selectorPath(call.Fun)
		if strings.HasSuffix(path, ".WaitReady") || path == "WaitReady" {
			waitsOnRegistry = true
			return true
		}
		// Polling primitives: readiness must be awaited, not sampled.
		if path == "time.NewTicker" || path == "time.Tick" || path == "time.Sleep" {
			issues = append(issues, SessionManagerIssue{
				File: parsed.RelPath,
				Line: parsed.Fset.Position(call.Pos()).Line,
				Rule: fmt.Sprintf("InitSessionIO must not busy-poll session readiness via %s (ADR-009)", path),
			})
		}
		return true
	})

	if !waitsOnRegistry {
		issues = append(issues, SessionManagerIssue{
			File: parsed.RelPath,
			Line: parsed.Fset.Position(fn.Pos()).Line,
			Rule: "InitSessionIO must wait via SessionRegistry.WaitReady (ADR-009)",
		})
	}
	return issues, nil
}
