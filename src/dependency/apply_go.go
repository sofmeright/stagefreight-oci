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

// goRunner executes a go subcommand in the given directory.
type goRunner func(ctx context.Context, dir string, args ...string) ([]byte, error)

// resolveGoRunner tries strategies in order and returns the first available go runner.
func resolveGoRunner(repoRoot string) (goRunner, error) {
	// Strategy 1: Native go binary in PATH
	if _, err := exec.LookPath("go"); err == nil {
		return nativeGoRunner, nil
	}
	// Strategy 2: Toolcache (STAGEFREIGHT_GO_HOME or /toolcache/go)
	if goHome := toolcacheGoHome(); goHome != "" {
		return toolcacheGoRunner(goHome), nil
	}
	// Strategy 3: Container runtime (docker/podman/nerdctl)
	if rt := detectContainerRuntime(); rt != "" {
		return containerGoRunner(rt, repoRoot), nil
	}
	// Strategy 4: Error
	return nil, fmt.Errorf("go toolchain not found: install Go, set STAGEFREIGHT_GO_HOME, or ensure a container runtime (docker/podman/nerdctl) is available")
}

func nativeGoRunner(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	return cmd.CombinedOutput()
}

func toolcacheGoRunner(goHome string) goRunner {
	goBin := filepath.Join(goHome, "bin", "go")
	return func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, goBin, args...)
		cmd.Dir = dir
		cmd.Env = os.Environ()
		return cmd.CombinedOutput()
	}
}

func toolcacheGoHome() string {
	if env := os.Getenv("STAGEFREIGHT_GO_HOME"); env != "" {
		if _, err := os.Stat(filepath.Join(env, "bin", "go")); err == nil {
			return env
		}
	}
	if _, err := os.Stat("/toolcache/go/bin/go"); err == nil {
		return "/toolcache/go"
	}
	return ""
}

func detectContainerRuntime() string {
	for _, rt := range []string{"docker", "podman", "nerdctl"} {
		if _, err := exec.LookPath(rt); err == nil {
			return rt
		}
	}
	return ""
}

func containerGoRunner(runtime, repoRoot string) goRunner {
	return func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		ver := parseGoVersion(dir, repoRoot)
		image := fmt.Sprintf("docker.io/library/golang:%s-alpine", ver)

		relDir, err := filepath.Rel(repoRoot, dir)
		if err != nil {
			relDir = "."
		}
		workDir := "/src"
		if relDir != "." && relDir != "" {
			workDir = "/src/" + filepath.ToSlash(relDir)
		}

		runArgs := []string{"run", "--rm", "-v", repoRoot + ":/src", "-w", workDir}
		// Pass through Go module-relevant environment variables
		for _, key := range []string{"GOPROXY", "GONOSUMDB", "GOPRIVATE", "GONOSUMCHECK", "GONOPROXY", "GOFLAGS"} {
			if val, ok := os.LookupEnv(key); ok {
				runArgs = append(runArgs, "-e", key+"="+val)
			}
		}

		runArgs = append(runArgs, image, "go")
		runArgs = append(runArgs, args...)

		cmd := exec.CommandContext(ctx, runtime, runArgs...)
		return cmd.CombinedOutput()
	}
}

// parseGoVersion extracts the go directive version from go.work or go.mod.
func parseGoVersion(dir, repoRoot string) string {
	// Prefer go.work at repo root (workspace mode)
	if ver := parseGoDirectiveFromFile(filepath.Join(repoRoot, "go.work")); ver != "" {
		return ver
	}
	// Try go.mod in module directory
	if ver := parseGoDirectiveFromFile(filepath.Join(dir, "go.mod")); ver != "" {
		return ver
	}
	// Try go.mod at repo root
	if dir != repoRoot {
		if ver := parseGoDirectiveFromFile(filepath.Join(repoRoot, "go.mod")); ver != "" {
			return ver
		}
	}
	return "1.24"
}

// parseGoDirectiveFromFile reads a go.mod or go.work file and returns the go version directive.
func parseGoDirectiveFromFile(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "go ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return fields[1]
			}
		}
	}
	return ""
}

// applyGoUpdates applies Go module dependency updates.
// Returns touched module dirs (repoRoot-relative) as the 3rd value —
// only dirs where go get + go mod tidy both succeeded.
func applyGoUpdates(ctx context.Context, deps []freshness.Dependency, repoRoot string) ([]AppliedUpdate, []SkippedDep, []string, error) {
	runGo, err := resolveGoRunner(repoRoot)
	if err != nil {
		return nil, nil, nil, err
	}

	// Check for go.work — workspace mode uses -C with relative paths
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

		// Determine working directory and build go get args
		var goDir string
		if hasWorkspace {
			goDir = repoRoot
		} else {
			goDir = modulePath
		}

		var goGetArgs []string
		if hasWorkspace {
			goGetArgs = append([]string{"-C", group.dir, "get"}, getArgs...)
		} else {
			goGetArgs = append([]string{"get"}, getArgs...)
		}

		out, err := runGo(ctx, goDir, goGetArgs...)
		if err != nil {
			return applied, skipped, nil, fmt.Errorf("go get in %s: %s\n%w", group.dir, string(out), err)
		}

		// go mod tidy
		var tidyArgs []string
		if hasWorkspace {
			tidyArgs = []string{"-C", group.dir, "mod", "tidy"}
		} else {
			tidyArgs = []string{"mod", "tidy"}
		}
		out, err = runGo(ctx, goDir, tidyArgs...)
		if err != nil {
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
