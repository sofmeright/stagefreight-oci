package lint

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	cacheDir       = ".stagefreight/cache/lint"
	engineVersion  = "0.1.0"
)

// Cache provides content-addressed lint result caching.
type Cache struct {
	RootDir string
	Enabled bool
}

// cacheEntry stores cached findings for a file+module combination.
type cacheEntry struct {
	Findings []Finding `json:"findings"`
}

// Key computes a cache key from file content, module name, and config.
func (c *Cache) Key(content []byte, moduleName string, configJSON string) string {
	h := sha256.New()
	h.Write(content)
	h.Write([]byte(moduleName))
	h.Write([]byte(configJSON))
	h.Write([]byte(engineVersion))
	return hex.EncodeToString(h.Sum(nil))
}

// Get retrieves cached findings. Returns nil, false on cache miss.
func (c *Cache) Get(key string) ([]Finding, bool) {
	if !c.Enabled {
		return nil, false
	}

	path := c.path(key)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}

	return entry.Findings, true
}

// Put stores findings in the cache.
func (c *Cache) Put(key string, findings []Finding) error {
	if !c.Enabled {
		return nil
	}

	path := c.path(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating cache dir: %w", err)
	}

	entry := cacheEntry{Findings: findings}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}

// Clear removes the entire cache directory.
func (c *Cache) Clear() error {
	dir := filepath.Join(c.RootDir, cacheDir)
	return os.RemoveAll(dir)
}

// path returns the filesystem path for a cache key.
// Uses 2-char prefix subdirectory to avoid huge flat directories.
func (c *Cache) path(key string) string {
	prefix := key[:2]
	return filepath.Join(c.RootDir, cacheDir, prefix, key+".json")
}

// EnsureGitignore adds .stagefreight/ to .gitignore if not already present.
func EnsureGitignore(rootDir string) {
	gitignorePath := filepath.Join(rootDir, ".gitignore")
	entry := ".stagefreight/"

	data, err := os.ReadFile(gitignorePath)
	if err == nil {
		// Check if already present
		for _, line := range splitLines(data) {
			if line == entry {
				return
			}
		}
	}

	// Append
	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return // best effort
	}
	defer f.Close()

	// Add newline before entry if file doesn't end with one
	if len(data) > 0 && data[len(data)-1] != '\n' {
		f.WriteString("\n")
	}
	f.WriteString(entry + "\n")
}

func splitLines(data []byte) []string {
	var lines []string
	start := 0
	for i, b := range data {
		if b == '\n' {
			line := string(data[start:i])
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, string(data[start:]))
	}
	return lines
}
