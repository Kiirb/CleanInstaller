// Package assets embeds files shipped inside the exe.
package assets

import "embed"

// Steam holds the shortcut artwork: grid.png, hero.png, logo.png, icon.png
// and logo.json (logo position).
//
//go:embed steam/*
var Steam embed.FS
