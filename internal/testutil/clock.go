// Package testutil holds test fakes: a clock that never sleeps and a
// recording HTTP server.
package testutil

import (
	"sync"
	"time"
)

// FakeClock is a clock for tests that never sleeps.
//
// After advances the clock by d at once and records d, so retry and rate
// limit waits complete immediately and can be asserted with Waits.
// AfterFunc timers (the per-attempt timeout) never fire on their own; a test
// fires them with FireTimers to simulate a timeout deterministically.
type FakeClock struct {
	mu     sync.Mutex
	now    time.Time
	waits  []time.Duration
	timers []*fakeTimer
}

type fakeTimer struct {
	f       func()
	stopped bool
}

// NewFakeClock returns a clock set to start.
func NewFakeClock(start time.Time) *FakeClock { return &FakeClock{now: start} }

// Now returns the fake time.
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// After records d, advances the clock by d and returns a channel that is
// already ready.
func (c *FakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.waits = append(c.waits, d)
	c.now = c.now.Add(d)
	ch := make(chan time.Time, 1)
	ch <- c.now
	return ch
}

// AfterFunc registers f; it runs only when FireTimers is called.
func (c *FakeClock) AfterFunc(_ time.Duration, f func()) func() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{f: f}
	c.timers = append(c.timers, t)
	return func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		was := !t.stopped
		t.stopped = true
		return was
	}
}

// FireTimers runs every pending AfterFunc timer and returns how many ran.
func (c *FakeClock) FireTimers() int {
	c.mu.Lock()
	var due []func()
	for _, t := range c.timers {
		if !t.stopped {
			t.stopped = true
			due = append(due, t.f)
		}
	}
	c.timers = nil
	c.mu.Unlock()
	for _, f := range due {
		f()
	}
	return len(due)
}

// Waits returns every duration passed to After, in order.
func (c *FakeClock) Waits() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]time.Duration(nil), c.waits...)
}
