package main

import (
	"context"
	"os"

	"workbench/internal/cli"
)

func main() {
	result := cli.Run(context.Background(), os.Args[1:], cli.Dependencies{})
	os.Exit(result.ExitCode)
}
