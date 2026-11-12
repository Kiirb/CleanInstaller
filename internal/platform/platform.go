// Package platform hides OS specifics (registry, processes, folders) behind
// one interface so the rest of the program can be tested and later ported.
package platform

import (
	"context"
	"os"
)

// Platform is everything OS-specific the installer and launcher need.
type Platform interface {
	SteamRoot() (string, error)
	DataDir() (string, error) // %LOCALAPPDATA%\CleanInstaller, created
	ProcessRunning(exePath string) bool
	SteamRunning() bool
	ShutdownSteam(ctx context.Context, steamRoot string) error
	StartSteam(steamRoot string) error
	OpenURL(url string) error
	OpenFolder(path string) error
	StartProcess(exe, dir string, args ...string) (*os.Process, error)
	// Lock takes the single-instance lock shared by installer and launcher.
	// ok is false if another CleanInstaller process holds it.
	Lock() (release func(), ok bool)
}

// AppName is the folder name under the per-user app data directory.
const AppName = "CleanInstaller"
