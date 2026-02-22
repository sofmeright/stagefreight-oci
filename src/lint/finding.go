package lint

import "fmt"

// Severity indicates how serious a finding is.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityCritical
)

func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarning:
		return "warning"
	case SeverityCritical:
		return "critical"
	default:
		return fmt.Sprintf("severity(%d)", int(s))
	}
}

// Finding represents a single lint result.
type Finding struct {
	File     string
	Line     int
	Column   int
	Module   string
	Severity Severity
	Message  string
}

// FileInfo is passed to each module for inspection.
type FileInfo struct {
	Path    string // relative path from repo root
	AbsPath string // absolute path on disk
	Size    int64
}
