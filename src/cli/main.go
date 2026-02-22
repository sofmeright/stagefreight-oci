package main

import (
	"os"

	"gitlab.prplanit.com/precisionplanit/stagefreight-oci/src/cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
