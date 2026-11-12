//go:build windows

package main

import "github.com/Kiirb/CleanInstaller/internal/platform"

func newPlatform() platform.Platform { return platform.Windows{} }
