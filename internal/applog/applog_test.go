package applog

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestApplogKeepsTen(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 11, 12, 15, 30, 0, 0, time.Local)
	for i := 0; i < 12; i++ {
		log, c, err := Open(dir, base.Add(time.Duration(i)*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		log.Info("hello", "i", i)
		_ = c.Close()
	}
	es, _ := os.ReadDir(dir)
	if len(es) != 10 {
		t.Fatalf("%d files", len(es))
	}
	if es[0].Name() != "cleaninstaller-20261112-153200.log" {
		t.Fatalf("oldest kept %s", es[0].Name())
	}
	b, _ := os.ReadFile(dir + "/" + es[9].Name())
	if !strings.Contains(string(b), "hello") || !strings.Contains(string(b), "i=11") {
		t.Fatalf("content %q", b)
	}
}
