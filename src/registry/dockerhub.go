package registry

import (
	"context"
	"fmt"
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
