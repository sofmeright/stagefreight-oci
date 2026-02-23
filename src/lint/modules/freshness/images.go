package freshness

import (
	"context"
	"fmt"
	"strings"

	"github.com/sofmeright/stagefreight/src/lint"
)

// dockerHubTagsResponse matches the Docker Hub v2 tags list API.
type dockerHubTagsResponse struct {
	Results []struct {
		Name string `json:"name"`
	} `json:"results"`
	Next string `json:"next"`
}

// checkImages resolves base image freshness for all FROM stages.
func (m *freshnessModule) checkImages(ctx context.Context, file lint.FileInfo, stages []stageInfo) []Dependency {
	var deps []Dependency

	for _, stage := range stages {
		// Skip scratch and build-stage references (no colon, no registry).
		if stage.Image == "scratch" {
			continue
		}
		// Skip inter-stage references (e.g. "FROM builder")
		if !strings.Contains(stage.Image, ":") && !strings.Contains(stage.Image, "/") {
			continue
		}

		dep := m.resolveImage(ctx, file.Path, stage)
		if dep != nil {
			deps = append(deps, *dep)
		}
	}

	return deps
}

// resolveImage queries Docker Hub for available tags and computes
// the version delta for a single base image.
func (m *freshnessModule) resolveImage(ctx context.Context, filePath string, stage stageInfo) *Dependency {
	image, tag := splitImageTag(stage.Image)
	if tag == "" {
		tag = "latest"
	}

	namespace, repo := splitImageNamespace(image)

	dep := &Dependency{
		Name:      stage.Image,
		Current:   tag,
		Ecosystem: EcosystemDockerImage,
		File:      filePath,
		Line:      stage.Line,
	}

	// Fetch tags — use custom registry if configured, else Docker Hub public API.
	ep := m.cfg.registryEndpoint(EcosystemDockerImage)
	defaultURL := fmt.Sprintf("https://registry.hub.docker.com/v2/repositories/%s/%s/tags?page_size=100&ordering=-last_updated", namespace, repo)
	url := m.cfg.registryURL(EcosystemDockerImage, defaultURL)
	if url != defaultURL {
		// Custom registry: use v2 tags/list endpoint.
		url = strings.TrimRight(url, "/") + fmt.Sprintf("/%s/%s/tags/list", namespace, repo)
	}
	dep.SourceURL = url

	var resp dockerHubTagsResponse
	if err := m.http.fetchJSON(ctx, url, &resp, ep); err != nil {
		// Network errors are non-fatal for freshness checks.
		return dep
	}

	tags := make([]string, 0, len(resp.Results))
	for _, r := range resp.Results {
		tags = append(tags, r.Name)
	}

	// Decompose current tag and find latest in same suffix family.
	current := decomposeTag(tag)
	if current.Version == nil {
		// Non-versioned tag (e.g. "latest", "noble") — can't do semver comparison.
		// Digest tracking handled by lockfile.go.
		return dep
	}

	family := filterTagsBySuffix(tags, current.Suffix)
	latest := latestInFamily(family)
	if latest == nil {
		return dep
	}

	dep.Latest = latest.Raw
	return dep
}

// splitImageTag splits "golang:1.25-alpine" into ("golang", "1.25-alpine").
func splitImageTag(ref string) (string, string) {
	// Handle digest references (image@sha256:...)
	if idx := strings.Index(ref, "@"); idx >= 0 {
		return ref[:idx], ""
	}
	// Find the last colon that isn't part of a port in the registry URL.
	// "registry:5000/image:tag" → split at last colon.
	lastColon := strings.LastIndex(ref, ":")
	if lastColon < 0 {
		return ref, ""
	}
	// If colon is before a slash, it's a port not a tag separator.
	afterColon := ref[lastColon+1:]
	if strings.Contains(afterColon, "/") {
		return ref, ""
	}
	return ref[:lastColon], afterColon
}

// splitImageNamespace splits an image name into (namespace, repo) for Docker Hub.
// "golang" → ("library", "golang")
// "prplanit/stagefreight" → ("prplanit", "stagefreight")
// "ghcr.io/owner/repo" → not Docker Hub, skip.
func splitImageNamespace(image string) (string, string) {
	// Strip registry prefix if present.
	// Docker Hub images may have docker.io/ prefix.
	image = strings.TrimPrefix(image, "docker.io/")
	image = strings.TrimPrefix(image, "index.docker.io/")

	parts := strings.SplitN(image, "/", 2)
	if len(parts) == 1 {
		return "library", parts[0]
	}
	// If the first part looks like a registry (contains a dot), this isn't Docker Hub.
	if strings.Contains(parts[0], ".") {
		return parts[0], parts[1]
	}
	return parts[0], parts[1]
}
