package markdown

import (
	"math/rand/v2"
	"testing"
)

// lcsLen is the textbook O(NM) LCS length.
func lcsLen(a, b []int) int {
	prev, cur := make([]int, len(b)+1), make([]int, len(b)+1)
	for i := range a {
		for j := range b {
			switch {
			case a[i] == b[j]:
				cur[j+1] = prev[j] + 1
			case prev[j+1] > cur[j]:
				cur[j+1] = prev[j+1]
			default:
				cur[j+1] = cur[j]
			}
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

func checkPairs(t *testing.T, a, b []int, pairs [][2]int) {
	t.Helper()
	li, lj := -1, -1
	for _, p := range pairs {
		if p[0] <= li || p[1] <= lj || a[p[0]] != b[p[1]] {
			t.Fatalf("bad pairs %v for %v / %v", pairs, a, b)
		}
		li, lj = p[0], p[1]
	}
}

func TestLCSIsLongest(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for range 5000 {
		a := make([]int, rng.IntN(30))
		b := make([]int, rng.IntN(30))
		alpha := 1 + rng.IntN(5)
		for i := range a {
			a[i] = rng.IntN(alpha)
		}
		for i := range b {
			b[i] = rng.IntN(alpha)
		}
		pairs := lcs(a, b)
		checkPairs(t, a, b, pairs)
		if want := lcsLen(a, b); len(pairs) != want {
			t.Fatalf("lcs(%v, %v) has %d pairs, want %d", a, b, len(pairs), want)
		}
	}
}

func TestLCSBounded(t *testing.T) {
	// Disjoint sequences need D = N+M, far past the bound: only the common
	// prefix and suffix match, and the call returns quickly.
	const n = 50000
	a, b := make([]int, n), make([]int, n)
	for i := range a {
		a[i], b[i] = i, n+i
	}
	a[0], b[0] = -1, -1
	a[n-1], b[n-1] = -2, -2
	pairs := lcs(a, b)
	checkPairs(t, a, b, pairs)
	if len(pairs) != 2 {
		t.Fatalf("got %d pairs, want prefix and suffix only", len(pairs))
	}
}
