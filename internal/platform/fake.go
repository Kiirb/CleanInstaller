package platform

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
)

// Fake is a scriptable Platform for tests in other packages.
type Fake struct {
	mu sync.Mutex

	Root    string
	RootErr error
	Data    string

	Running      map[string]bool // lower-case exe path -> running
	SteamUp      bool
	ShutdownErr  error
	StartProcErr error

	LockHeld           bool // true while someone holds the single-instance lock
	CheckLockOnStart   bool // record StartedWhileLocked if StartProcess runs with the lock held
	StartedWhileLocked bool

	OpenedURLs    []string
	OpenedFolders []string
	Started       []Started
	SteamStarted  int
	SteamShutdown int
}

// Started records one StartProcess call.
type Started struct {
	Exe, Dir string
	Args     []string
}

func (f *Fake) SteamRoot() (string, error) {
	if f.RootErr != nil {
		return "", f.RootErr
	}
	if f.Root == "" {
		return "", errors.New("steam not found")
	}
	return f.Root, nil
}

func (f *Fake) DataDir() (string, error) {
	return f.Data, os.MkdirAll(f.Data, 0o755)
}

func (f *Fake) ProcessRunning(exePath string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Running[strings.ToLower(exePath)]
}

func (f *Fake) SteamRunning() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.SteamUp
}

func (f *Fake) ShutdownSteam(ctx context.Context, steamRoot string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.SteamShutdown++
	if f.ShutdownErr != nil {
		return f.ShutdownErr
	}
	f.SteamUp = false
	return nil
}

func (f *Fake) StartSteam(steamRoot string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.SteamStarted++
	f.SteamUp = true
	return nil
}

func (f *Fake) OpenURL(url string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.OpenedURLs = append(f.OpenedURLs, url)
	return nil
}

func (f *Fake) OpenFolder(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.OpenedFolders = append(f.OpenedFolders, path)
	return nil
}

// StartProcess records the call and returns the current process as a stand-in
// (Wait on it is never called by code under test when Fake is used).
func (f *Fake) StartProcess(exe, dir string, args ...string) (*os.Process, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Started = append(f.Started, Started{exe, dir, args})
	if f.CheckLockOnStart && f.LockHeld {
		f.StartedWhileLocked = true
	}
	if f.StartProcErr != nil {
		return nil, f.StartProcErr
	}
	return nil, nil
}

// Lock simulates the single-instance mutex.
func (f *Fake) Lock() (func(), bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.LockHeld {
		return func() {}, false
	}
	f.LockHeld = true
	return func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.LockHeld = false
	}, true
}

var _ Platform = (*Fake)(nil)
