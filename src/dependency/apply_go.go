package dependency

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sofmeright/stagefreight/src/lint/modules/freshness"
)

// applyGoUpdates applies Go module dependency updates.
// Returns touched module dirs (repoRoot-relative) as the 3rd value —
// only dirs where go get + go mod tidy both succeeded.
func applyGoUpdates(ctx context.Context, deps []freshness.Dependency, repoRoot string) ([]AppliedUpdate, []SkippedDep, []string, error) {
	// Check for go.work — skip all Go updates in workspace context for v1
	// go.work: per-module only, no workspace sync
	hasWorkspace := false
	if _, err := os.Stat(filepath.Join(repoRoot, "go.work")); err == nil {
		hasWorkspace = true
	}

	// Group deps by module dir (derived from dep.File)
	type moduleGroup struct {
		dir  string // repoRoot-relative
		deps []freshness.Dependency
	}
	groupMap := make(map[string]*moduleGroup)
	for _, dep := range deps {
		dir := filepath.Dir(dep.File)
		if g, ok := groupMap[dir]; ok {
			g.deps = append(g.deps, dep)
		} else {
			groupMap[dir] = &moduleGroup{dir: dir, deps: []freshness.Dependency{dep}}
		}
	}

	var applied []AppliedUpdate
	var skipped []SkippedDep
	touchedSet := make(map[string]struct{})

	for _, group := range groupMap {
		modulePath := filepath.Join(repoRoot, group.dir)

		// Detect replace directives for this module
		replaceSet, err := detectReplaceDirectives(modulePath)
		if err != nil && len(replaceSet) == 0 {
			// Non-fatal — continue without replace detection
			replaceSet = nil
		}

		// Build batch go get args, skipping replaced modules
		var getArgs []string
		for _, dep := range group.deps {
			if replaceSet != nil && replaceSet[dep.Name] {
				skipped = append(skipped, SkippedDep{Dep: dep, Reason: "replace directive present"})
				continue
			}

			getArgs = append(getArgs, dep.Name+"@"+dep.Latest)
			update := AppliedUpdate{
				Dep:        dep,
				OldVer:     dep.Current,
				NewVer:     dep.Latest,
				UpdateType: updateType(dep.Current, dep.Latest),
			}
			for _, v := range dep.Vulnerabilities {
				update.CVEsFixed = append(update.CVEsFixed, v.ID)
			}
			applied = append(applied, update)
		}

		if len(getArgs) == 0 {
			continue
		}

		// Batch go get
		goGetArgs := []string{"get"}
		if hasWorkspace {
			goGetArgs = append([]string{"-C", modulePath, "get"}, getArgs...)
		} else {
			goGetArgs = append(goGetArgs, getArgs...)
		}

		cmd := exec.CommandContext(ctx, "go", goGetArgs...)
		if !hasWorkspace {
			cmd.Dir = modulePath
		}
		cmd.Env = os.Environ() // inherit GOPROXY, GONOSUMDB, GOPRIVATE
		if out, err := cmd.CombinedOutput(); err != nil {
			return applied, skipped, nil, fmt.Errorf("go get in %s: %s\n%w", group.dir, string(out), err)
		}

		// go mod tidy
		var tidyArgs []string
		if hasWorkspace {
			tidyArgs = []string{"-C", modulePath, "mod", "tidy"}
		} else {
			tidyArgs = []string{"mod", "tidy"}
		}
		tidyCmd := exec.CommandContext(ctx, "go", tidyArgs...)
		if !hasWorkspace {
			tidyCmd.Dir = modulePath
		}
		tidyCmd.Env = os.Environ()
		if out, err := tidyCmd.CombinedOutput(); err != nil {
			return applied, skipped, nil, fmt.Errorf("go mod tidy in %s: %s\n%w", group.dir, string(out), err)
		}

		// Both go get and go mod tidy succeeded — mark this dir as touched
		touchedSet[group.dir] = struct{}{}
	}

	touchedDirs := make([]string, 0, len(touchedSet))
	for d := range touchedSet {
		touchedDirs = append(touchedDirs, d)
	}
	sort.Strings(touchedDirs)
	return applied, skipped, touchedDirs, nil
}

// detectReplaceDirectives parses go.mod and returns a set of replaced module paths.
func detectReplaceDirectives(moduleDir string) (map[string]bool, error) {
	gomod := filepath.Join(moduleDir, "go.mod")
	f, err := os.Open(gomod)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	replaced := make(map[string]bool)
	scanner := bufio.NewScanner(f)
	inReplace := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "replace (") || strings.HasPrefix(line, "replace(") {
			inReplace = true
			continue
		}
		if inReplace {
			if line == ")" {
				inReplace = false
				continue
			}
			// Inside replace block: "module => replacement"
			parts := strings.Fields(line)
			if len(parts) >= 3 && parts[1] == "=>" {
				replaced[parts[0]] = true
			}
			continue
		}
		if strings.HasPrefix(line, "replace ") {
			// Single-line replace: "replace module => replacement"
			parts := strings.Fields(line)
			if len(parts) >= 4 && parts[2] == "=>" {
				replaced[parts[1]] = true
			}
		}
	}

	return replaced, scanner.Err()
}

// updateType determines the semver update type between two versions.
func updateType(current, latest string) string {
	delta := freshness.CompareDependencyVersions(current, latest, freshness.EcosystemGoMod)
	if delta.IsZero() {
		return "tag"
	}
	if delta.Major > 0 {
		return "major"
	}
	if delta.Minor > 0 {
		return "minor"
	}
	return "patch"
}
