package markdown

// Sibling diffing for base matching (LK_MARKDOWN.md §3.1). Work is bounded:
// the common prefix and suffix are matched in linear time, and the rest
// with Myers' O((N+M)·D) algorithm up to an edit distance that keeps both
// time and memory bounded. Past that bound the middle is left unmatched,
// and the caller pairs it by position (see pairHunk).

const (
	// diffCells bounds the work of one diff: (N+M)·D.
	diffCells = 1 << 24
	// diffMaxD bounds the edit distance Myers explores; the trace it keeps
	// for backtracking is about diffMaxD² int32s (4 MiB).
	diffMaxD = 1024
)

// lcs returns index pairs (i, j) with a[i] == b[j], strictly increasing in
// both, forming a longest common subsequence of a and b. When the middle of
// the inputs (after the common prefix and suffix) differs by more than the
// work bound allows, only the prefix and suffix are matched.
func lcs(a, b []int) [][2]int {
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	pairs := make([][2]int, 0, pre+suf)
	for i := range pre {
		pairs = append(pairs, [2]int{i, i})
	}
	for _, m := range myers(a[pre:len(a)-suf], b[pre:len(b)-suf]) {
		pairs = append(pairs, [2]int{m[0] + pre, m[1] + pre})
	}
	for k := suf; k > 0; k-- {
		pairs = append(pairs, [2]int{len(a) - k, len(b) - k})
	}
	return pairs
}

// myers returns the matched pairs of a shortest edit script from a to b,
// or nil when the edit distance exceeds the work bound.
func myers(a, b []int) [][2]int {
	n, m := len(a), len(b)
	if n == 0 || m == 0 {
		return nil
	}
	maxD := min(n+m, diffMaxD, max(1, diffCells/(n+m)))
	off := maxD + 1
	v := make([]int32, 2*maxD+3)
	trace := make([][]int32, 0, 16)
	for d := 0; d <= maxD; d++ {
		trace = append(trace, append([]int32(nil), v[off-d:off+d+1]...))
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[off+k-1] < v[off+k+1]) {
				x = int(v[off+k+1])
			} else {
				x = int(v[off+k-1]) + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[off+k] = int32(x)
			if x >= n && y >= m {
				return backtrack(trace, a, b)
			}
		}
	}
	return nil
}

// backtrack walks the trace from (len(a), len(b)) back to (0, 0) and
// returns the diagonal moves in order. trace[d] holds V as it was at the
// start of step d, indexed k+d.
func backtrack(trace [][]int32, a, b []int) [][2]int {
	x, y := len(a), len(b)
	var rev [][2]int
	for d := len(trace) - 1; d > 0; d-- {
		snap := trace[d]
		k := x - y
		prevK := k - 1
		if k == -d || (k != d && snap[k-1+d] < snap[k+1+d]) {
			prevK = k + 1
		}
		prevX := int(snap[prevK+d])
		prevY := prevX - prevK
		for x > prevX && y > prevY {
			x, y = x-1, y-1
			rev = append(rev, [2]int{x, y})
		}
		x, y = prevX, prevY
	}
	for x > 0 && y > 0 {
		x, y = x-1, y-1
		rev = append(rev, [2]int{x, y})
	}
	// Keep only valid matches, in order; a defect here must never pair
	// unequal nodes.
	out := make([][2]int, 0, len(rev))
	li, lj := -1, -1
	for i := len(rev) - 1; i >= 0; i-- {
		p := rev[i]
		if p[0] > li && p[1] > lj && p[0] < len(a) && p[1] < len(b) && p[0] >= 0 && p[1] >= 0 && a[p[0]] == b[p[1]] {
			out = append(out, p)
			li, lj = p[0], p[1]
		}
	}
	return out
}
