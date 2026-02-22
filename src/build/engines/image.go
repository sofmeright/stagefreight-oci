package engines

import (
	"context"
	"fmt"
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

	// Resolve version for tag templates
	versionInfo, _ := build.DetectVersion(det.RootDir)
	if versionInfo == nil {
		versionInfo = &build.VersionInfo{
			Version: "dev",
			SHA:     "unknown",
			Branch:  "unknown",
		}
	}

	// Build tags from registry configs
	var tags []string
	var registries []build.RegistryTarget

	if len(cfg.Registries) > 0 {
		for _, reg := range cfg.Registries {
			resolvedTags := build.ResolveTags(reg.Tags, versionInfo)
			target := build.RegistryTarget{
				URL:  reg.URL,
				Path: reg.Path,
				Tags: resolvedTags,
			}
			registries = append(registries, target)

			for _, t := range resolvedTags {
				ref := fmt.Sprintf("%s/%s:%s", reg.URL, reg.Path, t)
				tags = append(tags, ref)
			}
		}
	}

	step := build.BuildStep{
		Name:       "image",
		Dockerfile: dockerfile,
		Context:    buildContext,
		Target:     cfg.Target,
		Platforms:  platforms,
		BuildArgs:  cfg.BuildArgs,
		Tags:       tags,
		Output:     build.OutputImage,
		Registries: registries,
	}

	plan.Steps = append(plan.Steps, step)
	return plan, nil
}

func (e *imageEngine) Execute(ctx context.Context, plan *build.BuildPlan) (*build.BuildResult, error) {
	start := time.Now()
	result := &build.BuildResult{}

	bx := build.NewBuildx(false)

	for _, step := range plan.Steps {
		stepResult, err := bx.Build(ctx, step)
		result.Steps = append(result.Steps, *stepResult)
		if err != nil {
			result.Duration = time.Since(start)
			return result, err
		}
	}

	result.Duration = time.Since(start)
	return result, nil
}
