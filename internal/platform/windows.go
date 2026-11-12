//go:build windows

package platform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Windows is the real Platform on Windows.
type Windows struct{}

// SteamRoot reads HKCU\Software\Valve\Steam\SteamPath.
func (Windows) SteamRoot() (string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Valve\Steam`, registry.QUERY_VALUE)
	if err != nil {
		return "", fmt.Errorf("steam registry key: %w", err)
	}
	defer k.Close()
	p, _, err := k.GetStringValue("SteamPath")
	if err != nil || p == "" {
		return "", errors.New("steam path not set in registry")
	}
	p = filepath.Clean(p)
	if _, err := os.Stat(filepath.Join(p, "steam.exe")); err != nil {
		return "", fmt.Errorf("steam.exe not found in %s", p)
	}
	return p, nil
}

// DataDir is %LOCALAPPDATA%\CleanInstaller, created if missing.
func (Windows) DataDir() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return "", errors.New("LOCALAPPDATA not set")
	}
	d := filepath.Join(base, AppName)
	return d, os.MkdirAll(d, 0o755)
}

// processes returns the full image path of every process we may query.
func processes() []string {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var out []string
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		name := windows.UTF16ToString(e.ExeFile[:])
		full := name
		if h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, e.ProcessID); err == nil {
			buf := make([]uint16, windows.MAX_LONG_PATH)
			n := uint32(len(buf))
			if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) == nil {
				full = windows.UTF16ToString(buf[:n])
			}
			windows.CloseHandle(h)
		}
		out = append(out, full)
	}
	return out
}

// ProcessRunning reports whether a process with this full image path runs.
func (Windows) ProcessRunning(exePath string) bool {
	want := filepath.Clean(exePath)
	for _, p := range processes() {
		if strings.EqualFold(filepath.Clean(p), want) {
			return true
		}
	}
	return false
}

// SteamRunning reports whether any steam.exe is running.
func (Windows) SteamRunning() bool {
	for _, p := range processes() {
		if strings.EqualFold(filepath.Base(p), "steam.exe") {
			return true
		}
	}
	return false
}

// ShutdownSteam asks Steam to exit and waits until it has.
func (w Windows) ShutdownSteam(ctx context.Context, steamRoot string) error {
	if !w.SteamRunning() {
		return nil
	}
	if err := exec.Command(filepath.Join(steamRoot, "steam.exe"), "-shutdown").Start(); err != nil {
		return err
	}
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for w.SteamRunning() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
	// Steam writes its config files while exiting; give it a moment.
	time.Sleep(2 * time.Second)
	return nil
}

// StartSteam starts Steam normally.
func (w Windows) StartSteam(steamRoot string) error {
	_, err := w.StartProcess(filepath.Join(steamRoot, "steam.exe"), steamRoot)
	return err
}

// OpenURL opens a URL (including steam:// links) with its handler.
func (Windows) OpenURL(url string) error {
	return shellOpen(url)
}

// OpenFolder opens a folder in Explorer.
func (Windows) OpenFolder(path string) error {
	return shellOpen(path)
}

func shellOpen(target string) error {
	verb, _ := windows.UTF16PtrFromString("open")
	t, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, t, nil, nil, windows.SW_SHOWNORMAL)
}

// StartProcess starts exe in dir with args, detached from our console.
func (Windows) StartProcess(exe, dir string, args ...string) (*os.Process, error) {
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd.Process, nil
}

var _ Platform = Windows{}
