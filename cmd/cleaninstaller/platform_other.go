//go:build !windows

package main

import "github.com/Kiirb/CleanInstaller/internal/platform"

// Linux support is phase 2 (see the design document, section 12).
func newPlatform() platform.Platform { panic("this platform is not supported yet") }
