package registry

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// DockerHub implements the Registry interface for Docker Hub.
// Uses hub.docker.com v2 API for listing and deleting tags.
// Deletion requires a Personal Access Token (PAT) or password — the
// Docker Hub API does not support token-scoped deletion via registry v2.
type DockerHub struct {
	client   httpClient
	user     string
	pass     string
	jwtToken string
}

func NewDockerHub(user, pass string) *DockerHub {
	return &DockerHub{
		client: httpClient{
			base: "https://hub.docker.com",
		},
		user: user,
		pass: pass,
	}
}

func (d *DockerHub) Provider() string { return "dockerhub" }

// authenticate obtains a JWT token from Docker Hub. Cached for the session.
func (d *DockerHub) authenticate(ctx context.Context) error {
	if d.jwtToken != "" {
		return nil
	}
	if d.user == "" || d.pass == "" {
		return fmt.Errorf("dockerhub: credentials required for tag management")
	}

	var resp struct {
		Token string `json:"token"`
	}
	payload := map[string]string{
		"username": d.user,
		"password": d.pass,
	}

	_, err := d.client.doJSON(ctx, "POST", "https://hub.docker.com/v2/users/login/", payload, &resp)
	if err != nil {
		return fmt.Errorf("dockerhub: authentication failed: %w", err)
	}

	d.jwtToken = resp.Token
	d.client.headers = map[string]string{
		"Authorization": "JWT " + d.jwtToken,
	}
	return nil
}

func (d *DockerHub) ListTags(ctx context.Context, repo string) ([]TagInfo, error) {
	if err := d.authenticate(ctx); err != nil {
		return nil, err
	}

	var allTags []TagInfo
	url := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/tags/?page_size=100&ordering=-last_updated", repo)

	for url != "" {
		var page struct {
			Next    *string `json:"next"`
			Results []struct {
				Name        string    `json:"name"`
				Digest      string    `json:"digest"`
				LastUpdated time.Time `json:"last_updated"`
			} `json:"results"`
		}

		_, err := d.client.doJSON(ctx, "GET", url, nil, &page)
		if err != nil {
			return nil, fmt.Errorf("dockerhub: listing tags for %s: %w", repo, err)
		}

		for _, r := range page.Results {
			allTags = append(allTags, TagInfo{
				Name:      r.Name,
				Digest:    r.Digest,
				CreatedAt: r.LastUpdated,
			})
		}

		if page.Next != nil {
			url = *page.Next
		} else {
			url = ""
		}
	}

	return allTags, nil
}

func (d *DockerHub) DeleteTag(ctx context.Context, repo string, tag string) error {
	if err := d.authenticate(ctx); err != nil {
		return err
	}

	url := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/tags/%s/", repo, tag)
	_, err := d.client.doJSON(ctx, "DELETE", url, nil, nil)
	if err != nil {
		return fmt.Errorf("dockerhub: deleting tag %s/%s: %w", repo, tag, err)
	}
	return nil
}

func (d *DockerHub) UpdateDescription(ctx context.Context, repo, short, full string) error {
	if err := d.authenticate(ctx); err != nil {
		return err
	}

	// Docker Hub limits: 100 chars short, 25000 bytes full
	short = truncateAtWord(short, 100)
	if len(full) > 25000 {
		full = full[:25000]
	}

	payload := map[string]string{
		"description":      short,
		"full_description": full,
	}

	url := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/", repo)
	_, err := d.client.doJSON(ctx, "PATCH", url, payload, nil)
	if err != nil {
		return fmt.Errorf("dockerhub: updating description for %s: %w", repo, err)
	}
	return nil
}

// truncateAtWord truncates s at the last word boundary before maxLen.
func truncateAtWord(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	// Find last space before maxLen
	truncated := s[:maxLen]
	if idx := strings.LastIndexByte(truncated, ' '); idx > 0 {
		return truncated[:idx]
	}
	return truncated
}
