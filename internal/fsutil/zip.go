package fsutil

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ErrZipSlip means a zip entry would be written outside the destination.
var ErrZipSlip = errors.New("zip entry escapes destination")

// ExtractZip extracts zipPath into dst. If every entry sits under one common
// top-level folder, that folder is stripped. All entries are checked before
// anything is written. It returns the extracted files as forward-slash paths
// relative to dst.
func ExtractZip(zipPath, dst string, progress func(done, total int)) ([]string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	names := make([]string, len(zr.File))
	for i, f := range zr.File {
		n := strings.ReplaceAll(f.Name, `\`, "/")
		if err := checkEntry(n); err != nil {
			return nil, fmt.Errorf("%w: %q", ErrZipSlip, f.Name)
		}
		names[i] = n
	}
	prefix := commonRoot(names)

	var files []string
	for i, f := range zr.File {
		rel := strings.TrimPrefix(names[i], prefix)
		if rel == "" || strings.HasSuffix(rel, "/") {
			if rel != "" {
				if err := os.MkdirAll(filepath.Join(dst, filepath.FromSlash(rel)), 0o755); err != nil {
					return nil, err
				}
			}
		} else {
			if err := extractOne(f, filepath.Join(dst, filepath.FromSlash(rel))); err != nil {
				return nil, err
			}
			files = append(files, rel)
		}
		if progress != nil {
			progress(i+1, len(zr.File))
		}
	}
	return files, nil
}

func checkEntry(n string) error {
	if n == "" || strings.HasPrefix(n, "/") || strings.Contains(n, ":") {
		return ErrZipSlip
	}
	for _, el := range strings.Split(strings.TrimSuffix(n, "/"), "/") {
		if el == ".." || el == "" {
			return ErrZipSlip
		}
	}
	return nil
}

// commonRoot returns "Root/" if every entry is inside one top-level folder.
func commonRoot(names []string) string {
	root := ""
	for _, n := range names {
		i := strings.Index(n, "/")
		if i < 0 {
			return "" // a top-level file
		}
		r := n[:i+1]
		if root == "" {
			root = r
		} else if root != r {
			return ""
		}
	}
	return root
}

func extractOne(f *zip.File, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.Create(target)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
