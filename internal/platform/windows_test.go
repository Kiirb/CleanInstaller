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

func TestWindowsLockExclusive(t *testing.T) {
	release, ok := Windows{}.Lock()
	if !ok {
		t.Fatal("first lock failed")
	}
	if _, ok2 := (Windows{}).Lock(); ok2 {
		t.Fatal("second lock succeeded")
	}
	release()
	r2, ok3 := Windows{}.Lock()
	if !ok3 {
		t.Fatal("lock not released")
	}
	r2()
}
