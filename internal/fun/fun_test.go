package fun

import (
	"math/rand"
	"testing"
	"time"

	"github.com/Kiirb/CleanInstaller/internal/progress"
)

func TestJokeTextInterval(t *testing.T) {
	j := NewJokeText([]string{"a", "b", "c"}, rand.New(rand.NewSource(1)))
	t0 := time.Unix(1000, 0)
	first := j.Line(t0)
	if first == "" {
		t.Fatal("empty line")
	}
	if j.Line(t0.Add(1400*time.Millisecond)) != first {
		t.Fatal("changed before 1.5s")
	}
	prev := first
	now := t0
	for i := 0; i < 200; i++ {
		now = now.Add(3001 * time.Millisecond)
		l := j.Line(now)
		if l == prev {
			t.Fatalf("no change or immediate repeat after 3s at step %d", i)
		}
		prev = l
	}
}

func TestJokeTextSingleAndEmpty(t *testing.T) {
	if NewJokeText(nil, rand.New(rand.NewSource(1))).Line(time.Now()) != "" {
		t.Fatal("expected empty")
	}
	j := NewJokeText([]string{"only"}, rand.New(rand.NewSource(1)))
	if j.Line(time.Unix(0, 0)) != "only" || j.Line(time.Unix(10, 0)) != "only" {
		t.Fatal("single line")
	}
}

func TestChaosBarRangeAndFinish(t *testing.T) {
	c := NewChaosBar(rand.New(rand.NewSource(7)))
	prev := 0.0
	backwards := false
	for i := 0; i < 10000; i++ {
		v := c.Step(33*time.Millisecond, progress.Snapshot{Percent: float64(i) / 100})
		if v < 0 || v > 99 {
			t.Fatalf("out of range %v", v)
		}
		if v < prev-0.0001 {
			backwards = true
		}
		prev = v
	}
	if !backwards {
		t.Fatal("never went backwards")
	}
	if v := c.Step(33*time.Millisecond, progress.Snapshot{Percent: 100, Done: true}); v != 100 {
		t.Fatalf("finish %v", v)
	}
}
