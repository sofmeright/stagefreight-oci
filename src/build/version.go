package build

import "github.com/sofmeright/stagefreight/src/gitver"

// VersionInfo is an alias for backward compatibility.
type VersionInfo = gitver.VersionInfo

// DetectVersion delegates to the gitver package.
func DetectVersion(rootDir string) (*gitver.VersionInfo, error) {
	return gitver.DetectVersion(rootDir)
}
