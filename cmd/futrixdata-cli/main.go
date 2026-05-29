package main

import (
	"os"

	"futrixdata/platform/internal/cli"
)

func main() {
	runner := cli.NewRunner(os.Stdout, os.Stderr)
	os.Exit(runner.Run(os.Args[1:]))
}
