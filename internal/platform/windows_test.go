//go:build windows

package platform

import (
	"os"
	"strings"
	"testing"
)

func TestWindowsProcessRunningSelf(t *testing.T) {
	self, _ := os.Executable()
	w := Windows{}
	if !w.ProcessRunning(self) || !w.ProcessRunning(strings.ToUpper(self)) {
		t.Fatal("own process not found")
	}
	if w.ProcessRunning(self + ".nope") {
		t.Fatal("false positive")
	}
}

func TestWindowsDataDir(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	d, err := Windows{}.DataDir()
	if err != nil || !strings.HasSuffix(d, `\CleanInstaller`) {
		t.Fatalf("%v %s", err, d)
	}
	if st, err := os.Stat(d); err != nil || !st.IsDir() {
		t.Fatal("not created")
	}
}
