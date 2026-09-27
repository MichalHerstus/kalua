package main

import (
	"os"

	"kalua/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
