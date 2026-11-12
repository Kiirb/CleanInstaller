package main

import (
	"fmt"
	"os"
)

// version is overridden at build time via -ldflags "-X main.version=1.2.3".
var version = "0.0.0-dev"

func main() {
	mode, err := parseMode(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fmt.Println("CleanInstaller", version, "mode:", mode)
}
