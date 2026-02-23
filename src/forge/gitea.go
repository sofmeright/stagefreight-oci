package forge

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// GiteaForge implements the Forge interface for Gitea and Forgejo instances.
type GiteaForge struct {
	BaseURL string // e.g., "https://codeberg.org"
	Token   string
	Owner   string
	Repo    string
}

// NewGitea creates a Gitea/Forgejo forge client.
// Token is resolved from env: GITEA_TOKEN, FORGEJO_TOKEN.
// Owner/Repo is resolved from env: CI_REPO (Woodpecker CI) or
// GITHUB_REPOSITORY (Gitea Actions, which uses GitHub-compatible vars).
func NewGitea(baseURL string) *GiteaForge {
	token := os.Getenv("GITEA_TOKEN")
	if token == "" {
		token = os.Getenv("FORGEJO_TOKEN")
	}

	var owner, repo string

	// Woodpecker CI
	if ciRepo := os.Getenv("CI_REPO"); ciRepo != "" {
		if idx := strings.Index(ciRepo, "/"); idx >= 0 {
			owner = ciRepo[:idx]
			repo = ciRepo[idx+1:]
		}
	}

	// Gitea Actions (GitHub-compatible env vars)
	if owner == "" {
		if ghRepo := os.Getenv("GITHUB_REPOSITORY"); ghRepo != "" {
			if idx := strings.Index(ghRepo, "/"); idx >= 0 {
				owner = ghRepo[:idx]
				repo = ghRepo[idx+1:]
			}
		}
	}

	return &GiteaForge{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		Owner:   owner,
		Repo:    repo,
	}
}

func (g *GiteaForge) Provider() Provider { return Gitea }

func (g *GiteaForge) apiURL(path string) string {
	return fmt.Sprintf("%s/api/v1/repos/%s/%s%s", g.BaseURL, g.Owner, g.Repo, path)
}

func (g *GiteaForge) doJSON(ctx context.Context, method, url string, body interface{}, result interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+g.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("Gitea API %s %s: %d %s", method, url, resp.StatusCode, string(respBody))
	}

	if result != nil {
		return json.Unmarshal(respBody, result)
	}
	return nil
}

func (g *GiteaForge) CreateRelease(ctx context.Context, opts ReleaseOptions) (*Release, error) {
	payload := map[string]interface{}{
		"tag_name":   opts.TagName,
		"name":       opts.Name,
		"body":       opts.Description,
		"draft":      opts.Draft,
		"prerelease": opts.Prerelease,
	}

	var resp struct {
		ID      int    `json:"id"`
		HTMLURL string `json:"html_url"`
	}

	err := g.doJSON(ctx, "POST", g.apiURL("/releases"), payload, &resp)
	if err != nil {
		return nil, err
	}

	return &Release{
		ID:  fmt.Sprintf("%d", resp.ID),
		URL: resp.HTMLURL,
	}, nil
}

func (g *GiteaForge) UploadAsset(ctx context.Context, releaseID string, asset Asset) error {
	f, err := os.Open(asset.FilePath)
	if err != nil {
		return err
	}
	defer f.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("attachment", filepath.Base(asset.FilePath))
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, f); err != nil {
		return err
	}
	w.Close()

	uploadURL := g.apiURL(fmt.Sprintf("/releases/%s/assets", releaseID))
	req, err := http.NewRequestWithContext(ctx, "POST", uploadURL, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+g.Token)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Gitea upload asset: %d %s", resp.StatusCode, string(body))
	}
	return nil
}

func (g *GiteaForge) AddReleaseLink(ctx context.Context, releaseID string, link ReleaseLink) error {
	// Gitea doesn't have release links like GitLab.
	// Append the link to the release body instead.
	var rel struct {
		Body string `json:"body"`
	}
	if err := g.doJSON(ctx, "GET", g.apiURL("/releases/"+releaseID), nil, &rel); err != nil {
		return err
	}

	linkLine := fmt.Sprintf("- [%s](%s)", link.Name, link.URL)
	body := rel.Body
	if !strings.Contains(body, "### Container Images") {
		body += "\n\n### Container Images\n"
	}
	body += linkLine + "\n"

	payload := map[string]string{"body": body}
	return g.doJSON(ctx, "PATCH", g.apiURL("/releases/"+releaseID), payload, nil)
}

func (g *GiteaForge) CommitFile(ctx context.Context, opts CommitFileOptions) error {
	fileURL := g.apiURL("/contents/" + opts.Path)

	// Check if file exists to decide create vs update
	var existing struct {
		SHA string `json:"sha"`
	}
	existErr := g.doJSON(ctx, "GET", fileURL+"?ref="+opts.Branch, nil, &existing)

	payload := map[string]interface{}{
		"message": opts.Message,
		"content": base64.StdEncoding.EncodeToString(opts.Content),
		"branch":  opts.Branch,
	}

	if existErr == nil && existing.SHA != "" {
		// Update existing file (PUT)
		payload["sha"] = existing.SHA
		return g.doJSON(ctx, "PUT", fileURL, payload, nil)
	}

	// Create new file (POST)
	return g.doJSON(ctx, "POST", fileURL, payload, nil)
}

func (g *GiteaForge) CreateMR(ctx context.Context, opts MROptions) (*MR, error) {
	payload := map[string]interface{}{
		"title": opts.Title,
		"body":  opts.Description,
		"head":  opts.SourceBranch,
		"base":  opts.TargetBranch,
	}

	var resp struct {
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
	}

	err := g.doJSON(ctx, "POST", g.apiURL("/pulls"), payload, &resp)
	if err != nil {
		return nil, err
	}

	return &MR{
		ID:  fmt.Sprintf("%d", resp.Number),
		URL: resp.HTMLURL,
	}, nil
}
