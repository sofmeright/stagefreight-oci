package lint

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/sofmeright/stagefreight/src/config"
)

// Engine orchestrates lint modules across files.
type Engine struct {
	Config  config.LintConfig
	RootDir string
	Modules []Module
	Cache   *Cache
	Verbose bool

	CacheHits   atomic.Int64
	CacheMisses atomic.Int64
}

// NewEngine creates a lint engine with the selected modules.
func NewEngine(cfg config.LintConfig, rootDir string, moduleNames []string, skipNames []string, verbose bool, cache *Cache) (*Engine, error) {
	skipSet := make(map[string]bool, len(skipNames))
	for _, name := range skipNames {
		skipSet[name] = true
	}

	var modules []Module

	if len(moduleNames) > 0 {
		// Explicit module selection
		for _, name := range moduleNames {
			if skipSet[name] {
				continue
			}
			m, err := Get(name)
			if err != nil {
				return nil, err
			}
			modules = append(modules, m)
		}
	} else {
		// All default-enabled modules minus skipped
		for _, name := range All() {
			if skipSet[name] {
				continue
			}
			m, err := Get(name)
			if err != nil {
				return nil, err
			}

			// Check if config explicitly disables this module
			if mc, ok := cfg.Modules[name]; ok && mc.Enabled != nil && !*mc.Enabled {
				continue
			}

			if m.DefaultEnabled() {
				modules = append(modules, m)
			}
		}
	}

	if len(modules) == 0 {
		return nil, fmt.Errorf("no lint modules selected")
	}

	return &Engine{
		Config:  cfg,
		RootDir: rootDir,
		Modules: modules,
		Cache:   cache,
		Verbose: verbose,
	}, nil
}

// Run executes all modules against the given files and returns findings.
func (e *Engine) Run(ctx context.Context, files []FileInfo) ([]Finding, error) {
	var (
		mu       sync.Mutex
		findings []Finding
		wg       sync.WaitGroup
		errs     []error
	)

	for _, file := range files {
		if e.isExcluded(file.Path) {
			continue
		}

		// Read file content once for cache keying
		var content []byte
		if e.Cache != nil && e.Cache.Enabled {
			var err error
			content, err = os.ReadFile(file.AbsPath)
			if err != nil {
				// Non-fatal — run without cache for this file
				content = nil
			}
		}

		for _, mod := range e.Modules {
			wg.Add(1)
			go func(m Module, f FileInfo, data []byte) {
				defer wg.Done()

				// Check cache
				if e.Cache != nil && e.Cache.Enabled && data != nil {
					cfgJSON := e.moduleConfigJSON(m.Name())
					key := e.Cache.Key(data, m.Name(), cfgJSON)
					if cached, ok := e.Cache.Get(key); ok {
						e.CacheHits.Add(1)
						mu.Lock()
						findings = append(findings, cached...)
						mu.Unlock()
						return
					}
					e.CacheMisses.Add(1)

					// Run module and cache result
					results, err := m.Check(ctx, f)
					mu.Lock()
					defer mu.Unlock()
					if err != nil {
						errs = append(errs, fmt.Errorf("%s: %s: %w", m.Name(), f.Path, err))
						return
					}
					findings = append(findings, results...)
					// Cache even empty results (clean pass)
					if cacheErr := e.Cache.Put(key, results); cacheErr != nil && e.Verbose {
						fmt.Fprintf(os.Stderr, "cache: write failed for %s/%s: %v\n", m.Name(), f.Path, cacheErr)
					}
					return
				}

				// No cache — run directly
				results, err := m.Check(ctx, f)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					errs = append(errs, fmt.Errorf("%s: %s: %w", m.Name(), f.Path, err))
					return
				}
				findings = append(findings, results...)
			}(mod, file, content)
		}
	}

	wg.Wait()

	if len(errs) > 0 {
		return findings, fmt.Errorf("%d module errors (first: %w)", len(errs), errs[0])
	}

	return findings, nil
}

// CollectFiles walks the root directory and returns FileInfo for all regular files.
func (e *Engine) CollectFiles() ([]FileInfo, error) {
	var files []FileInfo

	err := filepath.WalkDir(e.RootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(e.RootDir, path)
		if err != nil {
			return err
		}

		// Skip hidden directories and .git
		if d.IsDir() {
			base := filepath.Base(rel)
			if strings.HasPrefix(base, ".") && base != "." {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip non-regular files
		if !d.Type().IsRegular() {
			return nil
		}

		if e.isExcluded(rel) {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		files = append(files, FileInfo{
			Path:    rel,
			AbsPath: path,
			Size:    info.Size(),
		})
		return nil
	})

	return files, err
}

func (e *Engine) isExcluded(path string) bool {
	for _, pattern := range e.Config.Exclude {
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
			return true
		}
	}
	return false
}

func (e *Engine) moduleConfigJSON(name string) string {
	mc, ok := e.Config.Modules[name]
	if !ok || mc.Options == nil {
		return "{}"
	}
	data, err := json.Marshal(mc.Options)
	if err != nil {
		return "{}"
	}
	return string(data)
}
