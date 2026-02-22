package version

import "fmt"

// These variables are injected at build time via -ldflags.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// String returns a human-readable version string.
func String() string {
	return fmt.Sprintf("stagefreight %s (%s, %s)", Version, Commit, BuildDate)
}
