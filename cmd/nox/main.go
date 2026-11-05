package main

import (
	"context"
	"os"

	"github.com/mapherez/nox-cli/internal/command"
)

func main() {
	os.Exit(command.Run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
