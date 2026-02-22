package main

import (
	"os"

	"github.com/sofmeright/stagefreight/src/cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
