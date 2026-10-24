//go:build !race

package guard

// slowdown scales wall-clock bounds in tests; the race detector is several
// times slower.
const slowdown = 1
