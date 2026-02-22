package lint

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Module is the interface every lint check implements.
type Module interface {
	Name() string
	Check(ctx context.Context, file FileInfo) ([]Finding, error)
	DefaultEnabled() bool
	AutoDetect() []string // glob patterns that trigger auto-enable
}

var (
	registryMu sync.RWMutex
	registry   = map[string]func() Module{}
)

// Register adds a module constructor to the global registry.
// Called from init() in each module file.
func Register(name string, constructor func() Module) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("lint: duplicate module registration: %s", name))
	}
	registry[name] = constructor
}

// Get returns a new instance of the named module.
func Get(name string) (Module, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	ctor, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("lint: unknown module: %s", name)
	}
	return ctor(), nil
}

// All returns sorted names of all registered modules.
func All() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
