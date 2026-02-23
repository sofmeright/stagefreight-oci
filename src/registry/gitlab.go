package registry

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

// GitLabRegistry implements the Registry interface for GitLab Container Registry.
// Uses the GitLab REST API (projects/:id/registry/repositories) for tag management.
// Requires either a CI_JOB_TOKEN (in CI) or a personal/project access token.
type GitLabRegistry struct {
	client    httpClient
	baseURL   string // e.g., "https://gitlab.example.com"
	projectID string // numeric ID or URL-encoded path
}

func NewGitLab(registryURL, user, pass string) *GitLabRegistry {
	// Resolve GitLab API base from environment or registry URL.
	// In CI, CI_SERVER_URL is the GitLab instance. Otherwise, try to
	// derive from the registry URL (registry.gitlab.com → gitlab.com).
	baseURL := os.Getenv("CI_SERVER_URL")
	if baseURL == "" {
		baseURL = deriveGitLabBase(registryURL)
	}

	projectID := os.Getenv("CI_PROJECT_ID")
	if projectID == "" {
		projectID = os.Getenv("CI_PROJECT_PATH")
	}

	// Token: prefer explicit pass, fall back to CI env vars.
	token := pass
	if token == "" {
		token = os.Getenv("GITLAB_TOKEN")
	}
	if token == "" {
		token = os.Getenv("CI_JOB_TOKEN")
	}

	return &GitLabRegistry{
		client: httpClient{
			base: baseURL,
			headers: map[string]string{
				"PRIVATE-TOKEN": token,
			},
		},
		baseURL:   baseURL,
		projectID: projectID,
	}
}

func (g *GitLabRegistry) Provider() string { return "gitlab" }

func (g *GitLabRegistry) apiURL(path string) string {
	return fmt.Sprintf("%s/api/v4/projects/%s%s", g.baseURL, url.PathEscape(g.projectID), path)
}

// findRepositoryID looks up the registry repository ID for a given image path.
func (g *GitLabRegistry) findRepositoryID(ctx context.Context, repo string) (int, error) {
	var repos []struct {
		ID   int    `json:"id"`
		Path string `json:"path"`
	}

	_, err := g.client.doJSON(ctx, "GET", g.apiURL("/registry/repositories"), nil, &repos)
	if err != nil {
		return 0, fmt.Errorf("gitlab: listing registry repositories: %w", err)
	}

	for _, r := range repos {
		if r.Path == repo {
			return r.ID, nil
		}
	}

	return 0, fmt.Errorf("gitlab: repository %q not found in project %s", repo, g.projectID)
}

func (g *GitLabRegistry) ListTags(ctx context.Context, repo string) ([]TagInfo, error) {
	repoID, err := g.findRepositoryID(ctx, repo)
	if err != nil {
		return nil, err
	}

	var allTags []TagInfo
	page := 1

	for {
		var tags []struct {
			Name      string    `json:"name"`
			CreatedAt time.Time `json:"created_at"`
			Digest    string    `json:"digest"`
		}

		url := g.apiURL(fmt.Sprintf("/registry/repositories/%d/tags?per_page=100&page=%d", repoID, page))
		_, err := g.client.doJSON(ctx, "GET", url, nil, &tags)
		if err != nil {
			return nil, fmt.Errorf("gitlab: listing tags for %s: %w", repo, err)
		}

		if len(tags) == 0 {
			break
		}

		for _, t := range tags {
			allTags = append(allTags, TagInfo{
				Name:      t.Name,
				Digest:    t.Digest,
				CreatedAt: t.CreatedAt,
			})
		}

		page++
	}

	return allTags, nil
}

func (g *GitLabRegistry) DeleteTag(ctx context.Context, repo string, tag string) error {
	repoID, err := g.findRepositoryID(ctx, repo)
	if err != nil {
		return err
	}

	url := g.apiURL(fmt.Sprintf("/registry/repositories/%d/tags/%s", repoID, url.PathEscape(tag)))
	_, err = g.client.doJSON(ctx, "DELETE", url, nil, nil)
	if err != nil {
		return fmt.Errorf("gitlab: deleting tag %s/%s: %w", repo, tag, err)
	}
	return nil
}

// deriveGitLabBase attempts to derive the GitLab API base URL from a registry URL.
// e.g., "registry.gitlab.com" → "https://gitlab.com"
//
//	"registry.gitlab.example.com" → "https://gitlab.example.com"
func deriveGitLabBase(registryURL string) string {
	host := strings.ToLower(registryURL)
	if idx := strings.Index(host, "://"); idx >= 0 {
		host = host[idx+3:]
	}
	if idx := strings.IndexByte(host, '/'); idx >= 0 {
		host = host[:idx]
	}

	// Strip "registry." prefix
	host = strings.TrimPrefix(host, "registry.")
	return "https://" + host
}
