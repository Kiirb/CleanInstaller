// Package fun implements the installer's joke mode: random status lines and
// a nonsense progress bar. The lines themselves only ever come from the
// manifest; none are compiled into the program.
package fun

import (
	"math/rand"
	"time"

	"github.com/Kiirb/CleanInstaller/internal/progress"
)

// JokeText rotates through lines every 1.5–3 seconds.
type JokeText struct {
	lines []string
	rng   *rand.Rand
	cur   int
	next  time.Time
}

// NewJokeText creates a rotator over lines.
func NewJokeText(lines []string, rng *rand.Rand) *JokeText {
	return &JokeText{lines: lines, rng: rng, cur: -1}
}

// Line returns the line to show at now.
func (j *JokeText) Line(now time.Time) string {
	if len(j.lines) == 0 {
		return ""
	}
	if j.cur < 0 || !now.Before(j.next) {
		n := j.rng.Intn(len(j.lines))
		if len(j.lines) > 1 && n == j.cur {
			n = (n + 1 + j.rng.Intn(len(j.lines)-1)) % len(j.lines)
		}
		j.cur = n
		j.next = now.Add(1500*time.Millisecond + time.Duration(j.rng.Int63n(int64(1500*time.Millisecond))))
	}
	return j.lines[j.cur]
}

// ChaosBar produces a progress value that speeds up, stalls and jumps back,
// and snaps to 100 the moment the real work is done.
type ChaosBar struct {
	rng      *rand.Rand
	value    float64 // percent
	velocity float64 // percent per second
	until    time.Duration
}

// NewChaosBar creates a chaos bar.
func NewChaosBar(rng *rand.Rand) *ChaosBar {
	return &ChaosBar{rng: rng, velocity: 10}
}

// Step advances by dt and returns the value to draw.
func (c *ChaosBar) Step(dt time.Duration, real progress.Snapshot) float64 {
	if real.Done {
		c.value = 100
		return 100
	}
	c.until -= dt
	if c.until <= 0 {
		// Pick a new mood: crawl, sprint, stall or reverse.
		switch c.rng.Intn(5) {
		case 0:
			c.velocity = 0
		case 1:
			c.velocity = -(5 + c.rng.Float64()*20)
		case 2:
			c.velocity = 30 + c.rng.Float64()*50
		default:
			c.velocity = 2 + c.rng.Float64()*10
		}
		c.until = time.Duration(500+c.rng.Intn(2500)) * time.Millisecond
		if c.rng.Intn(6) == 0 {
			c.value -= 10 + c.rng.Float64()*30 // sudden setback
		}
	}
	c.value += c.velocity * dt.Seconds()
	if c.value < 0 {
		c.value = 0
	}
	if c.value > 99 {
		c.value = 99
		c.velocity = -(5 + c.rng.Float64()*20)
	}
	return c.value
}
