// Command manifestgen resolves sources.json into manifest.json. It runs in
// the scheduled GitHub Actions workflow only.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/Kiirb/CleanInstaller/internal/ghrel"
	"github.com/Kiirb/CleanInstaller/internal/manifest"
	"github.com/Kiirb/CleanInstaller/internal/manifestgen"
)

func main() {
	srcPath := flag.String("sources", "sources.json", "path to sources.json")
	manPath := flag.String("manifest", "manifest.json", "path to manifest.json")
	flag.Parse()
	os.Exit(run(*srcPath, *manPath))
}

func run(srcPath, manPath string) int {
	srcBytes, err := os.ReadFile(srcPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	src, err := manifest.ParseSources(srcBytes)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	old := &manifest.Manifest{Schema: manifest.SchemaVersion}
	if b, err := os.ReadFile(manPath); err == nil {
		if old, err = manifest.Parse(b); err != nil {
			fmt.Fprintln(os.Stderr, "existing manifest:", err)
			return 2
		}
	} else if !os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	httpc := &http.Client{Timeout: 5 * time.Minute}
	res, err := manifestgen.Build(ctx, src, old,
		manifestgen.ClientResolver{C: &ghrel.Client{HTTP: httpc, Token: os.Getenv("GITHUB_TOKEN")}},
		manifestgen.HTTPHasher{HTTP: httpc}, time.Now())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if len(res.Changes) > 0 {
		b, err := manifest.Marshal(res.Manifest)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		if err := os.WriteFile(manPath, b, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	}
	for _, c := range res.Changes {
		fmt.Println("- " + c)
	}
	for _, f := range res.Failures {
		fmt.Fprintln(os.Stderr, "FAILED: "+f)
	}
	if len(res.Failures) > 0 {
		return 1
	}
	return 0
}
