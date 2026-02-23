package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Local implements the Registry interface for the local Docker daemon.
// Uses `docker` CLI commands to list and remove images — no remote API calls.
// This enables retention for local dev builds loaded with --load.
type Local struct{}

func NewLocal() *Local {
	return &Local{}
}

func (l *Local) Provider() string { return "local" }

func (l *Local) ListTags(ctx context.Context, repo string) ([]TagInfo, error) {
	// docker images outputs JSON with repository, tag, ID, and created timestamp.
	cmd := exec.CommandContext(ctx, "docker", "images",
		"--format", `{"repository":"{{.Repository}}","tag":"{{.Tag}}","id":"{{.ID}}","created":"{{.CreatedAt}}"}`,
		"--filter", fmt.Sprintf("reference=%s", repo),
		"--no-trunc",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("local: docker images: %w: %s", err, stderr.String())
	}

	var tags []TagInfo
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if line == "" {
			continue
		}

		var img struct {
			Repository string `json:"repository"`
			Tag        string `json:"tag"`
			ID         string `json:"id"`
			Created    string `json:"created"`
		}
		if err := json.Unmarshal([]byte(line), &img); err != nil {
			continue
		}

		// Skip <none> tags
		if img.Tag == "<none>" {
			continue
		}

		// Parse docker's timestamp format: "2025-02-22 15:30:45 +0000 UTC"
		created := parseDockerTimestamp(img.Created)

		tags = append(tags, TagInfo{
			Name:      img.Tag,
			Digest:    img.ID,
			CreatedAt: created,
		})
	}

	return tags, nil
}

func (l *Local) DeleteTag(ctx context.Context, repo string, tag string) error {
	ref := fmt.Sprintf("%s:%s", repo, tag)
	cmd := exec.CommandContext(ctx, "docker", "rmi", ref)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("local: docker rmi %s: %w: %s", ref, err, stderr.String())
	}
	return nil
}

// parseDockerTimestamp parses the various timestamp formats docker images outputs.
func parseDockerTimestamp(s string) time.Time {
	// Docker outputs different formats depending on version/platform.
	formats := []string{
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05 +0000 UTC",
		"2006-01-02T15:04:05Z",
		time.RFC3339,
	}
	for _, f := range formats {
		if t, err := time.Parse(f, strings.TrimSpace(s)); err == nil {
			return t
		}
	}
	return time.Time{}
}
