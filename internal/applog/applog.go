// Package applog writes one log file per run and keeps the newest ten.
package applog

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const keep = 10

// Open creates logDir/cleaninstaller-YYYYMMDD-HHMMSS.log and deletes the
// oldest files beyond ten.
func Open(logDir string, now time.Time) (*slog.Logger, io.Closer, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, nil, err
	}
	name := "cleaninstaller-" + now.Format("20060102-150405") + ".log"
	f, err := os.OpenFile(filepath.Join(logDir, name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, nil, err
	}
	prune(logDir)
	return slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug})), f, nil
}

func prune(dir string) {
	es, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var logs []string
	for _, e := range es {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "cleaninstaller-") && strings.HasSuffix(e.Name(), ".log") {
			logs = append(logs, e.Name())
		}
	}
	sort.Strings(logs) // timestamped names sort chronologically
	for len(logs) > keep {
		_ = os.Remove(filepath.Join(dir, logs[0])) // best effort; a locked old log is harmless
		logs = logs[1:]
	}
}

// Discard is a logger that drops everything (tests, early startup).
func Discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
