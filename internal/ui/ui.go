// Package ui defines the windows the installer and launcher show. Flows talk
// to the UI interface; the Gio implementation draws it.
package ui

import "github.com/Kiirb/CleanInstaller/internal/progress"

// Display selects joke behaviour for the progress window. The zero value is
// honest progress.
type Display struct {
	JokeText  bool
	ChaosBar  bool
	JokeLines []string
}

// UI is everything a flow can ask of the user. Calls block until answered.
type UI interface {
	// Progress shows the progress window and returns immediately; the window
	// polls t until t.Snapshot().Done, another dialog, or Close.
	Progress(title string, t *progress.Tracker, d Display)
	Confirm(title, body, okLabel, cancelLabel string) bool
	Choose(title, body string, options []string) (index int, ok bool)
	Info(title, body string, buttons ...string) int
	Error(err error, logDir string)
	Close()
}
