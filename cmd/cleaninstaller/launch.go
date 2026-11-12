package main

import (
	"context"
	"log/slog"

	"github.com/Kiirb/CleanInstaller/internal/install"
	"github.com/Kiirb/CleanInstaller/internal/launch"
)

func runLaunch(ctx context.Context, d install.Deps, log *slog.Logger) error {
	return launch.Run(ctx, d, version)
}
