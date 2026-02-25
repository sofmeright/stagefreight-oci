package lint

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// cacheSchemaVersion is bumped whenever the cache entry format or
// finding semantics change. This invalidates stale entries automatically.
// Bump this when: finding fields change, severity logic changes, or
// module output format changes.
const cacheSchemaVersion = "1"

// Cache provides content-addressed lint result caching.
// Dir is the resolved cache directory (call ResolveCacheDir to compute it).
type Cache struct {
	Dir     string
	Enabled bool
}

// cacheEntry stores cached findings for a file+module combination.
type cacheEntry struct {
	Findings []Finding `json:"findings"`
	CachedAt int64     `json:"cached_at,omitempty"`
}

// ResolveCacheDir determines the cache directory using the following precedence:
//  1. STAGEFREIGHT_CACHE_DIR env var (used as-is, caller controls the path)
//  2. configDir from .stagefreight.yml cache_dir (resolved relative to rootDir)
//  3. os.UserCacheDir()/stagefreight/<project-hash>/lint (XDG-aware default)
//
// The project hash is a truncated SHA-256 of the absolute rootDir path,
// keeping per-project caches isolated without long nested directory names.
func ResolveCacheDir(rootDir string, configDir string) string {
	// 1. Env var takes priority
	if dir := os.Getenv("STAGEFREIGHT_CACHE_DIR"); dir != "" {
		return filepath.Join(dir, "lint")
	}

	// 2. Config-specified directory (relative to project root)
	if configDir != "" {
		if filepath.IsAbs(configDir) {
			return filepath.Join(configDir, "lint")
		}
		return filepath.Join(rootDir, configDir, "lint")
	}

	// 3. XDG-aware default via os.UserCacheDir
	base, err := os.UserCacheDir()
	if err != nil {
		// Last resort: temp directory
		base = os.TempDir()
	}

	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		absRoot = rootDir
	}
	h := sha256.Sum256([]byte(absRoot))
	projectHash := hex.EncodeToString(h[:])[:12]

	return filepath.Join(base, "stagefreight", projectHash, "lint")
}

// Key computes a cache key from file content, module name, and config.
func (c *Cache) Key(content []byte, moduleName string, configJSON string) string {
	h := sha256.New()
	h.Write(content)
	h.Write([]byte(moduleName))
	h.Write([]byte(configJSON))
	h.Write([]byte(cacheSchemaVersion))
	return hex.EncodeToString(h.Sum(nil))
}

// Get retrieves cached findings. Returns nil, false on cache miss.
// maxAge controls TTL: 0 means no expiry (content-only modules),
// >0 expires entries older than the duration (external-state modules).
func (c *Cache) Get(key string, maxAge time.Duration) ([]Finding, bool) {
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
		os.Remove(path) // self-heal corrupted entries
		return nil, false
	}

	// TTL check for external-state modules.
	// Old entries without a timestamp (CachedAt==0) are treated as
	// expired — prevents pre-TTL cache files from being served forever.
	if maxAge > 0 {
		if entry.CachedAt == 0 {
			return nil, false
		}
		if time.Since(time.Unix(entry.CachedAt, 0)) > maxAge {
			return nil, false
		}
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

	entry := cacheEntry{Findings: findings, CachedAt: time.Now().Unix()}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}

// Clear removes the entire cache directory.
func (c *Cache) Clear() error {
	return os.RemoveAll(c.Dir)
}

// path returns the filesystem path for a cache key.
// Uses 2-char prefix subdirectory to avoid huge flat directories.
func (c *Cache) path(key string) string {
	prefix := key[:2]
	return filepath.Join(c.Dir, prefix, key+".json")
}
