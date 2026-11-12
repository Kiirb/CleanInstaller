package progress

import (
	"math"
	"sync"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestTrackerWeighted(t *testing.T) {
	tr := NewTracker()
	tr.Plan(40, 60)
	tr.Begin("copy")
	tr.Set(0.5)
	if s := tr.Snapshot(); !near(s.Percent, 20) || s.Status != "copy" || s.Done {
		t.Fatalf("%+v", s)
	}
	tr.Begin("download")
	tr.Set(0.5)
	if s := tr.Snapshot(); !near(s.Percent, 70) {
		t.Fatalf("%+v", s)
	}
	tr.Finish()
	if s := tr.Snapshot(); !near(s.Percent, 100) || !s.Done {
		t.Fatalf("%+v", s)
	}
}

func TestTrackerNormalisesAndClamps(t *testing.T) {
	tr := NewTracker()
	tr.Plan(1, 1, 2)
	tr.Begin("a")
	tr.Set(2) // clamped to 1
	if s := tr.Snapshot(); !near(s.Percent, 25) {
		t.Fatalf("%+v", s)
	}
	tr.Set(0.1) // monotonic within stage
	if s := tr.Snapshot(); !near(s.Percent, 25) {
		t.Fatalf("went backwards: %+v", s)
	}
	tr.Begin("b")
	tr.Begin("c")
	if s := tr.Snapshot(); !near(s.Percent, 50) {
		t.Fatalf("%+v", s)
	}
	tr.Begin("extra") // beyond plan: stays below 100 until Finish
	if s := tr.Snapshot(); s.Percent > 100 || s.Done {
		t.Fatalf("%+v", s)
	}
}

func TestTrackerStatus(t *testing.T) {
	tr := NewTracker()
	tr.Plan(1)
	tr.Begin("x")
	tr.Status("y")
	if tr.Snapshot().Status != "y" {
		t.Fatal("status not updated")
	}
}

func TestTrackerConcurrent(t *testing.T) {
	tr := NewTracker()
	tr.Plan(1)
	tr.Begin("x")
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i <= 1000; i++ {
			tr.Set(float64(i) / 1000)
		}
	}()
	for i := 0; i < 1000; i++ {
		_ = tr.Snapshot()
	}
	wg.Wait()
	if !near(tr.Snapshot().Percent, 100) {
		t.Fatal(tr.Snapshot())
	}
}
