package main

import (
	"github.com/theorchestradev/orchestra/internal/core"
	"os"
)

func main() { os.Exit(core.RunCLI(os.Args[1:], os.Stdout, os.Stderr)) }
