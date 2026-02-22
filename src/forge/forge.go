// Package forge provides a platform-agnostic abstraction over git forges
// (GitLab, GitHub, Gitea/Forgejo). Every write operation (release creation,
// badge update, file commit, MR/PR creation) goes through this interface
// so StageFreight works identically regardless of where the repo is hosted.
package forge

import "context"

// Provider identifies a git forge platform.
type Provider string

const (
	GitLab  Provider = "gitlab"
	GitHub  Provider = "github"
	Gitea   Provider = "gitea"
	Unknown Provider = "unknown"
)

// Forge is the interface every platform implements.
type Forge interface {
	// Provider returns which platform this forge represents.
	Provider() Provider

	// CreateRelease creates a release/tag on the forge.
	CreateRelease(ctx context.Context, opts ReleaseOptions) (*Release, error)

	// UploadAsset attaches a file to an existing release.
	UploadAsset(ctx context.Context, releaseID string, asset Asset) error

	// AddReleaseLink adds a URL link to a release (e.g., registry image links).
	AddReleaseLink(ctx context.Context, releaseID string, link ReleaseLink) error

	// CommitFile creates or updates a file in the repo via the forge API.
	// Used for badge SVG updates without a local clone.
	CommitFile(ctx context.Context, opts CommitFileOptions) error

	// CreateMR opens a merge/pull request.
	CreateMR(ctx context.Context, opts MROptions) (*MR, error)
}

// ReleaseOptions configures a new release.
type ReleaseOptions struct {
	TagName     string
	Name        string
	Description string // markdown body (release notes)
	Draft       bool
	Prerelease  bool
}

// Release is a created release on a forge.
type Release struct {
	ID  string // platform-specific ID
	URL string // web URL to the release page
}

// Asset is a file to attach to a release.
type Asset struct {
	Name     string // display name
	FilePath string // local file to upload
	MIMEType string // e.g., "application/json"
}

// ReleaseLink is a URL to embed in a release (e.g., registry image link).
type ReleaseLink struct {
	Name     string // display name (e.g., "Docker Hub 1.3.0")
	URL      string // target URL
	LinkType string // "image", "package", "other"
}

// CommitFileOptions configures a file commit via forge API.
type CommitFileOptions struct {
	Branch  string
	Path    string // file path in repo
	Content []byte
	Message string
}

// MROptions configures a merge/pull request.
type MROptions struct {
	Title        string
	Description  string
	SourceBranch string
	TargetBranch string
	Draft        bool
}

// MR is a created merge/pull request.
type MR struct {
	ID  string
	URL string
}
