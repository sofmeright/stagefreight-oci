package gitver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DockerHubInfo holds metadata fetched from the Docker Hub API.
type DockerHubInfo struct {
	Pulls  int64  // total pull count
	Stars  int    // star count
	Size   int64  // compressed size of latest tag in bytes
	Latest string // digest of latest tag (sha256:...)
}

// FetchDockerHubInfo retrieves repository metadata from Docker Hub.
// namespace/repo format: "prplanit/stagefreight".
func FetchDockerHubInfo(namespace, repo string) (*DockerHubInfo, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	info := &DockerHubInfo{}

	// Fetch repository info (pulls, stars).
	repoURL := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/%s/", namespace, repo)
	resp, err := client.Get(repoURL)
	if err != nil {
		return nil, fmt.Errorf("docker hub repo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("docker hub repo: %s", resp.Status)
	}

	var repoData struct {
		PullCount int64 `json:"pull_count"`
		StarCount int   `json:"star_count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&repoData); err != nil {
		return nil, fmt.Errorf("docker hub repo decode: %w", err)
	}
	info.Pulls = repoData.PullCount
	info.Stars = repoData.StarCount

	// Fetch latest tag info (size, digest).
	tagURL := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/%s/tags/latest", namespace, repo)
	tagResp, err := client.Get(tagURL)
	if err == nil {
		defer tagResp.Body.Close()
		if tagResp.StatusCode == http.StatusOK {
			var tagData struct {
				FullSize int64 `json:"full_size"`
				Digest   string `json:"digest"`
				Images   []struct {
					Size   int64  `json:"size"`
					Digest string `json:"digest"`
				} `json:"images"`
			}
			if err := json.NewDecoder(tagResp.Body).Decode(&tagData); err == nil {
				info.Size = tagData.FullSize
				info.Latest = tagData.Digest
				// If no top-level size, sum from images.
				if info.Size == 0 && len(tagData.Images) > 0 {
					for _, img := range tagData.Images {
						info.Size += img.Size
					}
				}
				if info.Latest == "" && len(tagData.Images) > 0 {
					info.Latest = tagData.Images[0].Digest
				}
			}
		}
	}

	return info, nil
}

// ResolveDockerTemplates replaces {docker.*} templates with values from Docker Hub.
// Returns s unchanged if info is nil or no {docker.} templates are present.
func ResolveDockerTemplates(s string, info *DockerHubInfo) string {
	if info == nil || !strings.Contains(s, "{docker.") {
		return s
	}

	s = strings.ReplaceAll(s, "{docker.pulls:raw}", strconv.FormatInt(info.Pulls, 10))
	s = strings.ReplaceAll(s, "{docker.pulls}", formatCount(info.Pulls))
	s = strings.ReplaceAll(s, "{docker.stars}", strconv.Itoa(info.Stars))
	s = strings.ReplaceAll(s, "{docker.size:raw}", strconv.FormatInt(info.Size, 10))
	s = strings.ReplaceAll(s, "{docker.size}", formatBytes(info.Size))
	s = strings.ReplaceAll(s, "{docker.latest}", shortDigest(info.Latest))

	return s
}

// formatCount formats a number for human display: 1247 → "1.2k", 1234567 → "1.2M".
func formatCount(n int64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", float64(n)/1_000_000_000)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return strconv.FormatInt(n, 10)
	}
}

// formatBytes formats bytes for human display: 75890432 → "72.4 MB".
func formatBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// shortDigest returns the first 12 hex characters of a sha256:... digest.
func shortDigest(digest string) string {
	digest = strings.TrimPrefix(digest, "sha256:")
	if len(digest) > 12 {
		return digest[:12]
	}
	return digest
}
