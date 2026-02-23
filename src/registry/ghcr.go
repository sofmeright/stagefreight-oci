package registry

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// GHCR implements the Registry interface for GitHub Container Registry (ghcr.io).
// Uses the GitHub REST API for package version management.
// Requires a PAT with read:packages and delete:packages scopes.
type GHCR struct {
	client httpClient
	user   string
}

func NewGHCR(user, pass string) *GHCR {
	return &GHCR{
		client: httpClient{
			base: "https://api.github.com",
			headers: map[string]string{
				"Authorization":        "Bearer " + pass,
				"Accept":               "application/vnd.github+json",
				"X-GitHub-Api-Version": "2022-11-28",
			},
		},
		user: user,
	}
}

func (g *GHCR) Provider() string { return "ghcr" }

// splitRepo splits "owner/image" into owner and package name.
// GHCR repos are referenced as "owner/image" in the path field.
func splitRepo(repo string) (owner, pkg string, isOrg bool) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return repo, repo, false
	}
	return parts[0], parts[1], true
}

func (g *GHCR) ListTags(ctx context.Context, repo string) ([]TagInfo, error) {
	owner, pkg, _ := splitRepo(repo)

	var allTags []TagInfo
	page := 1

	for {
		var versions []struct {
			ID        int    `json:"id"`
			Name      string `json:"name"`
			CreatedAt string `json:"created_at"`
			Metadata  struct {
				Container struct {
					Tags []string `json:"tags"`
				} `json:"container"`
			} `json:"metadata"`
		}

		// Try user endpoint first; if owner matches authenticated user, use /user/
		// Otherwise use /orgs/ endpoint. We try user first as it covers personal accounts.
		apiURL := fmt.Sprintf("https://api.github.com/users/%s/packages/container/%s/versions?per_page=100&page=%d",
			url.PathEscape(owner), url.PathEscape(pkg), page)

		_, err := g.client.doJSON(ctx, "GET", apiURL, nil, &versions)
		if err != nil {
			// Fallback to org endpoint
			apiURL = fmt.Sprintf("https://api.github.com/orgs/%s/packages/container/%s/versions?per_page=100&page=%d",
				url.PathEscape(owner), url.PathEscape(pkg), page)
			_, err = g.client.doJSON(ctx, "GET", apiURL, nil, &versions)
			if err != nil {
				return nil, fmt.Errorf("ghcr: listing versions for %s: %w", repo, err)
			}
		}

		if len(versions) == 0 {
			break
		}

		for _, v := range versions {
			created, _ := time.Parse(time.RFC3339, v.CreatedAt)
			// Each version can have multiple tags; emit one TagInfo per tag.
			for _, tag := range v.Metadata.Container.Tags {
				allTags = append(allTags, TagInfo{
					Name:      tag,
					Digest:    v.Name, // version name is the digest
					CreatedAt: created,
				})
			}
		}

		page++
	}

	return allTags, nil
}

func (g *GHCR) DeleteTag(ctx context.Context, repo string, tag string) error {
	// GHCR deletes by version ID, not by tag directly.
	// We need to find the version ID for this tag first.
	owner, pkg, _ := splitRepo(repo)

	versionID, err := g.findVersionID(ctx, owner, pkg, tag)
	if err != nil {
		return err
	}

	// Try user endpoint, fall back to org
	apiURL := fmt.Sprintf("https://api.github.com/users/%s/packages/container/%s/versions/%d",
		url.PathEscape(owner), url.PathEscape(pkg), versionID)

	_, err = g.client.doJSON(ctx, "DELETE", apiURL, nil, nil)
	if err != nil {
		apiURL = fmt.Sprintf("https://api.github.com/orgs/%s/packages/container/%s/versions/%d",
			url.PathEscape(owner), url.PathEscape(pkg), versionID)
		_, err = g.client.doJSON(ctx, "DELETE", apiURL, nil, nil)
		if err != nil {
			return fmt.Errorf("ghcr: deleting tag %s in %s: %w", tag, repo, err)
		}
	}

	return nil
}

func (g *GHCR) UpdateDescription(_ context.Context, _, _, _ string) error { return nil }

func (g *GHCR) findVersionID(ctx context.Context, owner, pkg, tag string) (int, error) {
	page := 1
	for {
		var versions []struct {
			ID       int `json:"id"`
			Metadata struct {
				Container struct {
					Tags []string `json:"tags"`
				} `json:"container"`
			} `json:"metadata"`
		}

		apiURL := fmt.Sprintf("https://api.github.com/users/%s/packages/container/%s/versions?per_page=100&page=%d",
			url.PathEscape(owner), url.PathEscape(pkg), page)

		_, err := g.client.doJSON(ctx, "GET", apiURL, nil, &versions)
		if err != nil {
			// Try org endpoint
			apiURL = fmt.Sprintf("https://api.github.com/orgs/%s/packages/container/%s/versions?per_page=100&page=%d",
				url.PathEscape(owner), url.PathEscape(pkg), page)
			_, err = g.client.doJSON(ctx, "GET", apiURL, nil, &versions)
			if err != nil {
				return 0, fmt.Errorf("ghcr: finding version for tag %s in %s/%s: %w", tag, owner, pkg, err)
			}
		}

		if len(versions) == 0 {
			break
		}

		for _, v := range versions {
			for _, t := range v.Metadata.Container.Tags {
				if t == tag {
					return v.ID, nil
				}
			}
		}

		page++
	}

	return 0, fmt.Errorf("ghcr: tag %q not found in %s/%s", tag, owner, pkg)
}
