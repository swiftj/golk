package lk

import (
	"math/rand/v2"
	"time"
)

// clock is the time source for timeouts, backoff and rate limiting. Tests
// inject a fake so they never sleep.
type clock interface {
	Now() time.Time
	// After returns a channel that receives once d has elapsed.
	After(d time.Duration) <-chan time.Time
	// AfterFunc calls f once d has elapsed, unless the returned stop is called first.
	AfterFunc(d time.Duration, f func()) (stop func() bool)
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
func (realClock) AfterFunc(d time.Duration, f func()) func() bool {
	return time.AfterFunc(d, f).Stop
}

// randFloat returns a pseudo-random number in [0, 1). It must be safe for
// concurrent use; the default, math/rand/v2's global source, is.
type randFloat func() float64

var defaultRand randFloat = rand.Float64
