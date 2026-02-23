package registry

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Gitea implements the Registry interface for Gitea/Forgejo container package registries.
// Uses the Gitea package API (/api/v1/packages/:owner).
// Requires a token with package:read and package:delete scopes.
type Gitea struct {
	client  httpClient
	baseURL string
}

func NewGitea(registryURL, user, pass string) *Gitea {
	base := normalizeURL(registryURL)

	headers := map[string]string{}
	if pass != "" && user != "" {
		headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
	} else if pass != "" {
		headers["Authorization"] = "token " + pass
	}

	return &Gitea{
		client: httpClient{
			base:    base,
			headers: headers,
		},
		baseURL: base,
	}
}

func (g *Gitea) Provider() string { return "gitea" }

func (g *Gitea) ListTags(ctx context.Context, repo string) ([]TagInfo, error) {
	// repo format: "owner/package" (e.g., "myorg/myapp")
	owner, pkg := splitGiteaRepo(repo)

	var allTags []TagInfo
	page := 1

	for {
		var versions []struct {
			ID        int64     `json:"id"`
			Version   string    `json:"version"`
			CreatedAt time.Time `json:"created_at"`
		}

		apiURL := fmt.Sprintf("%s/api/v1/packages/%s?type=container&q=%s&page=%d&limit=50",
			g.baseURL, url.PathEscape(owner), url.PathEscape(pkg), page)

		_, err := g.client.doJSON(ctx, "GET", apiURL, nil, &versions)
		if err != nil {
			return nil, fmt.Errorf("gitea: listing packages for %s: %w", repo, err)
		}

		if len(versions) == 0 {
			break
		}

		for _, v := range versions {
			allTags = append(allTags, TagInfo{
				Name:      v.Version,
				CreatedAt: v.CreatedAt,
			})
		}

		page++
	}

	return allTags, nil
}

func (g *Gitea) DeleteTag(ctx context.Context, repo string, tag string) error {
	owner, pkg := splitGiteaRepo(repo)

	apiURL := fmt.Sprintf("%s/api/v1/packages/%s/container/%s/%s",
		g.baseURL, url.PathEscape(owner), url.PathEscape(pkg), url.PathEscape(tag))

	_, err := g.client.doJSON(ctx, "DELETE", apiURL, nil, nil)
	if err != nil {
		return fmt.Errorf("gitea: deleting tag %s in %s: %w", tag, repo, err)
	}
	return nil
}

// splitGiteaRepo splits "owner/package" into owner and package name.
func splitGiteaRepo(repo string) (owner, pkg string) {
	idx := strings.IndexByte(repo, '/')
	if idx < 0 {
		return repo, repo
	}
	return repo[:idx], repo[idx+1:]
}
