//go:build race

package markdown

// slowdown scales wall-clock bounds in tests; the race detector is several
// times slower.
const slowdown = 20
