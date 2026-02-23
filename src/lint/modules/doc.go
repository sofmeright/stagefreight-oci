// Package modules contains all built-in lint modules.
// Import this package to register all modules via their init() functions.
package modules

import (
	// Register the freshness sub-package module.
	_ "github.com/sofmeright/stagefreight/src/lint/modules/freshness"
)
