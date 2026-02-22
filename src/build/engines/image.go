package engines

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"gitlab.prplanit.com/precisionplanit/stagefreight-oci/src/build"
	"gitlab.prplanit.com/precisionplanit/stagefreight-oci/src/config"
)

func init() {
	build.Register("image", func() build.Engine { return &imageEngine{} })
}

// imageEngine builds container images and pushes to registries.
type imageEngine struct{}

func (e *imageEngine) Name() string { return "image" }

func (e *imageEngine) Detect(ctx context.Context, rootDir string) (*build.Detection, error) {
	return build.DetectRepo(rootDir)
}

func (e *imageEngine) Plan(ctx context.Context, cfgRaw interface{}, det *build.Detection) (*build.BuildPlan, error) {
	cfg, ok := cfgRaw.(*config.DockerConfig)
	if !ok {
		return nil, fmt.Errorf("image engine: expected *config.DockerConfig, got %T", cfgRaw)
	}

	plan := &build.BuildPlan{}

	// Resolve Dockerfile path
	dockerfile := cfg.Dockerfile
	if dockerfile == "" && len(det.Dockerfiles) > 0 {
		dockerfile = det.Dockerfiles[0].Path
	}
	if dockerfile == "" {
		return nil, fmt.Errorf("no Dockerfile found")
	}

	// Resolve context
	buildContext := cfg.Context
	if buildContext == "" {
		buildContext = "."
	}

	// Resolve platforms
	platforms := cfg.Platforms
	if len(platforms) == 0 {
		platforms = []string{fmt.Sprintf("linux/%s", runtime.GOARCH)}
	}

	// Resolve version for templates (tags, paths, URLs)
	versionInfo, _ := build.DetectVersion(det.RootDir)
	if versionInfo == nil {
		versionInfo = &build.VersionInfo{
			Version: "dev",
			Base:    "0.0.0",
			SHA:     "unknown",
			Branch:  "unknown",
		}
	}

	// Resolve current branch and tag for registry filtering
	currentBranch := resolveBranch(det, versionInfo)
	currentGitTag := os.Getenv("CI_COMMIT_TAG")

	// Build tags from registry configs, filtering by branch and git tag
	var tags []string
	var registries []build.RegistryTarget

	if len(cfg.Registries) > 0 {
		for _, reg := range cfg.Registries {
			if !registryAllowed(reg, currentBranch, currentGitTag) {
				continue
			}

			// Resolve templates in URL, path, and tags
			resolvedURL := build.ResolveTemplate(reg.URL, versionInfo)
			resolvedPath := build.ResolveTemplate(reg.Path, versionInfo)
			resolvedTags := build.ResolveTags(reg.Tags, versionInfo)

			// Resolve provider: explicit config, or auto-detect from URL
			provider := reg.Provider
			if provider == "" {
				provider = build.DetectProvider(resolvedURL)
			}

			target := build.RegistryTarget{
				URL:         resolvedURL,
				Path:        resolvedPath,
				Tags:        resolvedTags,
				Credentials: reg.Credentials,
				Provider:    provider,
			}
			registries = append(registries, target)

			for _, t := range resolvedTags {
				ref := fmt.Sprintf("%s/%s:%s", resolvedURL, resolvedPath, t)
				tags = append(tags, ref)
			}
		}
	}

	// Auto-inject standard build args when the Dockerfile declares matching ARGs
	// and no explicit override exists in the config.
	buildArgs := cfg.BuildArgs
	if buildArgs == nil {
		buildArgs = map[string]string{}
	}
	buildArgs = autoInjectBuildArgs(buildArgs, det, versionInfo, dockerfile)

	step := build.BuildStep{
		Name:       "image",
		Dockerfile: dockerfile,
		Context:    buildContext,
		Target:     cfg.Target,
		Platforms:  platforms,
		BuildArgs:  buildArgs,
		Tags:       tags,
		Output:     build.OutputImage,
		Registries: registries,
	}

	plan.Steps = append(plan.Steps, step)
	return plan, nil
}

// resolveBranch determines the current branch from detection, version info,
// or CI environment variables (for detached HEAD in CI).
func resolveBranch(det *build.Detection, v *build.VersionInfo) string {
	// 1. Git detection
	if det.GitInfo != nil && det.GitInfo.Branch != "" {
		return det.GitInfo.Branch
	}

	// 2. Version info (uses git rev-parse which may work when detection doesn't)
	if v != nil && v.Branch != "" && v.Branch != "HEAD" {
		return v.Branch
	}

	// 3. CI env vars (detached HEAD in CI)
	if b := os.Getenv("CI_COMMIT_BRANCH"); b != "" {
		return b
	}
	if b := os.Getenv("GITHUB_REF_NAME"); b != "" {
		return b
	}

	return ""
}

// registryAllowed checks if the current branch and git tag permit pushing to a registry.
// Uses the standard MatchPatterns from config — supports regex and ! negation.
// Empty patterns = no filter (always allowed).
func registryAllowed(reg config.RegistryConfig, branch, gitTag string) bool {
	if !config.MatchPatterns(reg.Branches, branch) {
		return false
	}
	if gitTag != "" && !config.MatchPatterns(reg.GitTags, gitTag) {
		return false
	}
	return true
}

// autoInjectBuildArgs adds VERSION, COMMIT, and BUILD_DATE build args when the
// Dockerfile declares matching ARGs and no explicit override is set.
func autoInjectBuildArgs(existing map[string]string, det *build.Detection, v *build.VersionInfo, dockerfilePath string) map[string]string {
	if v == nil {
		return existing
	}

	// Find the parsed Dockerfile info that matches our chosen Dockerfile
	var dfArgs []string
	for _, df := range det.Dockerfiles {
		if df.Path == dockerfilePath {
			dfArgs = df.Args
			break
		}
	}
	if len(dfArgs) == 0 {
		return existing
	}

	// Build a set of Dockerfile ARG names for fast lookup
	argSet := make(map[string]bool, len(dfArgs))
	for _, a := range dfArgs {
		argSet[a] = true
	}

	// Inject if Dockerfile declares the ARG and no explicit override exists
	if argSet["VERSION"] {
		if _, ok := existing["VERSION"]; !ok {
			existing["VERSION"] = v.Version
		}
	}
	if argSet["COMMIT"] {
		if _, ok := existing["COMMIT"]; !ok {
			existing["COMMIT"] = v.SHA
		}
	}
	if argSet["BUILD_DATE"] {
		if _, ok := existing["BUILD_DATE"]; !ok {
			existing["BUILD_DATE"] = time.Now().UTC().Format(time.RFC3339)
		}
	}

	return existing
}

func (e *imageEngine) Execute(ctx context.Context, plan *build.BuildPlan) (*build.BuildResult, error) {
	start := time.Now()
	result := &build.BuildResult{}

	bx := build.NewBuildx(false)

	// Authenticate to registries before building
	for _, step := range plan.Steps {
		if step.Push && len(step.Registries) > 0 {
			if err := bx.Login(ctx, step.Registries); err != nil {
				return result, err
			}
			break // all steps share the same daemon auth state
		}
	}

	for _, step := range plan.Steps {
		stepResult, err := bx.Build(ctx, step)
		result.Steps = append(result.Steps, *stepResult)
		if err != nil {
			result.Duration = time.Since(start)
			return result, err
		}

		// Save image tarball for downstream scanning/attestation
		if step.SavePath != "" && len(step.Tags) > 0 {
			if err := bx.Save(ctx, step.Tags[0], step.SavePath); err != nil {
				return result, err
			}
		}
	}

	result.Duration = time.Since(start)
	return result, nil
}
