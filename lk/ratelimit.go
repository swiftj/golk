package lk

import (
	"context"
	"time"

	"golang.org/x/time/rate"
)

// RateLimits configures client-side pacing. The API's per-key minute tiers are
// 300 requests and 120 writes (POST, PUT, PATCH, DELETE); see
// DefaultRateLimits for the defaults.
type RateLimits struct {
	// RequestsPerMinute paces every request. A write counts here too.
	RequestsPerMinute float64
	// WritesPerMinute paces POST, PUT, PATCH and DELETE.
	WritesPerMinute float64
	// RequestBurst and WriteBurst are how many requests may go out at once
	// before pacing starts. Values below 1 are treated as 1.
	RequestBurst int
	WriteBurst   int
}

// DefaultRateLimits stays about 17% under the API's per-key minute tiers.
// The hour, day and per-project tiers are not paced client-side; a 429 from
// them is retried as described in the SDK guide.
var DefaultRateLimits = RateLimits{
	RequestsPerMinute: 250,
	WritesPerMinute:   100,
	RequestBurst:      10,
	WriteBurst:        5,
}

// RateLimiter paces requests. One limiter can be shared by several clients
// (WithRateLimiter) so that together they stay under one key's limits.
type RateLimiter struct {
	all    *rate.Limiter
	writes *rate.Limiter
}

// NewRateLimiter returns a limiter for l.
func NewRateLimiter(l RateLimits) *RateLimiter {
	return &RateLimiter{
		all:    rate.NewLimiter(perMinute(l.RequestsPerMinute), max(l.RequestBurst, 1)),
		writes: rate.NewLimiter(perMinute(l.WritesPerMinute), max(l.WriteBurst, 1)),
	}
}

func perMinute(n float64) rate.Limit {
	if n <= 0 {
		return rate.Inf
	}
	return rate.Limit(n / 60)
}

func isWrite(method string) bool { return method != "GET" && method != "HEAD" }

// wait blocks until one request (and, for a write, one write) may go out.
func (l *RateLimiter) wait(ctx context.Context, clk clock, method string) error {
	if l == nil {
		return nil
	}
	now := clk.Now()
	rs := []*rate.Reservation{l.all.ReserveN(now, 1)}
	if isWrite(method) {
		rs = append(rs, l.writes.ReserveN(now, 1))
	}
	var delay time.Duration
	for _, r := range rs {
		delay = max(delay, r.DelayFrom(now))
	}
	if delay == 0 {
		return nil
	}
	select {
	case <-clk.After(delay):
		return nil
	case <-ctx.Done():
		at := clk.Now()
		for _, r := range rs {
			r.CancelAt(at)
		}
		return context.Cause(ctx)
	}
}
