package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Kiirb/CleanInstaller/assets"
	"github.com/Kiirb/CleanInstaller/internal/applog"
	"github.com/Kiirb/CleanInstaller/internal/download"
	"github.com/Kiirb/CleanInstaller/internal/install"
	"github.com/Kiirb/CleanInstaller/internal/ui"
)

// version is overridden at build time via -ldflags "-X main.version=1.2.3".
var version = "0.0.0-dev"

const manifestURL = "https://raw.githubusercontent.com/Kiirb/CleanInstaller/main/manifest.json"

func main() {
	mode, err := parseMode(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	p := newPlatform()
	download.UserAgent = "CleanInstaller/" + version

	logDir := os.TempDir()
	log := applog.Discard()
	if dataDir, err := p.DataDir(); err == nil {
		logDir = filepath.Join(dataDir, "logs")
		if l, c, err := applog.Open(logDir, time.Now()); err == nil {
			log = l
			defer c.Close()
		}
	}
	log.Info("start", "version", version, "mode", mode.String(), "args", os.Args[1:])

	self, err := os.Executable()
	if err != nil {
		log.Error("os.Executable", "err", err)
	}
	d := install.Deps{
		P:           p,
		HTTP:        httpClient(),
		ManifestURL: manifestURL,
		SelfPath:    self,
		Log:         log,
		TempDir:     os.TempDir(),
		Art:         assets.Steam,
	}

	ui.Run("CleanInstaller", p.OpenFolder, func(u ui.UI) {
		d.UI = u
		ctx := context.Background()
		var err error
		switch mode {
		case ModeInstall:
			var out install.Outcome
			out, err = install.Run(ctx, d)
			log.Info("installer finished", "outcome", int(out))
		case ModeLaunch:
			err = runLaunch(ctx, d, log)
		}
		if err != nil {
			log.Error("failed", "err", err)
			u.Error(err, logDir)
		}
	})
}

// httpClient has connect/header timeouts but no overall timeout, because the
// mod pack is ~100 MB.
func httpClient() *http.Client {
	return &http.Client{Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       60 * time.Second,
	}}
}
