package modules

import (
	"context"
	"fmt"

	"gitlab.prplanit.com/precisionplanit/stagefreight-oci/src/lint"
)

const defaultLargeFileMax int64 = 500 * 1024 // 500 KB

func init() {
	lint.Register("largefiles", func() lint.Module { return &largeFilesModule{maxBytes: defaultLargeFileMax} })
}

type largeFilesModule struct {
	maxBytes int64
}

func (m *largeFilesModule) Name() string        { return "largefiles" }
func (m *largeFilesModule) DefaultEnabled() bool { return true }
func (m *largeFilesModule) AutoDetect() []string { return nil }

func (m *largeFilesModule) Check(ctx context.Context, file lint.FileInfo) ([]lint.Finding, error) {
	if file.Size <= m.maxBytes {
		return nil, nil
	}

	return []lint.Finding{
		{
			File:     file.Path,
			Module:   m.Name(),
			Severity: lint.SeverityWarning,
			Message:  fmt.Sprintf("file size %s exceeds threshold %s", humanSize(file.Size), humanSize(m.maxBytes)),
		},
	}, nil
}

func humanSize(b int64) string {
	switch {
	case b >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
	case b >= 1024:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	default:
		return fmt.Sprintf("%d B", b)
	}
}
