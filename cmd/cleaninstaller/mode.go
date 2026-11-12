package main

import "fmt"

// Mode selects what the exe does on this run.
type Mode int

const (
	// ModeInstall runs the installer (no arguments).
	ModeInstall Mode = iota + 1
	// ModeLaunch runs the launcher (started from the Steam shortcut).
	ModeLaunch
)

func (m Mode) String() string {
	switch m {
	case ModeInstall:
		return "install"
	case ModeLaunch:
		return "launch"
	}
	return fmt.Sprintf("Mode(%d)", int(m))
}

// parseMode maps command-line arguments to a Mode. The mode is never inferred
// from files on disk.
func parseMode(args []string) (Mode, error) {
	switch {
	case len(args) == 0:
		return ModeInstall, nil
	case len(args) == 1 && args[0] == "--launch":
		return ModeLaunch, nil
	}
	return 0, fmt.Errorf("unknown arguments: %q", args)
}
