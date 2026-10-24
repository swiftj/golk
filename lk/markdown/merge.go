package markdown

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"

	"github.com/swiftj/golk/lk/blocks"
)

// reconcile matches parsed against base (LK_MARKDOWN.md §3.1) and applies the
// id rules M-ID-01..03, M-SAFE-04 and M-SAFE-06. base may be nil. parsed is
// modified in place (it receives base ids and hidden fields) and becomes
// part of the result; base is never modified or aliased. On error it
// returns no blocks.
func reconcile(parsed, base blocks.List, h hooks, allowRemoveOpaque bool) (blocks.List, *Report, error) {
	if h.key == nil || h.opaque == nil {
		return nil, nil, errors.New("markdown: base matching needs key and opaque hooks")
	}
	r := &reconciler{
		h:         h,
		claimOf:   map[blocks.Node]blocks.Node{},
		claimedBy: map[blocks.Node]blocks.Node{},
		used:      map[blocks.Node]bool{},
		status:    map[blocks.Node]status{},
	}
	if err := r.claim(parsed, base); err != nil {
		return nil, nil, err
	}
	nodes, err := r.level(blockNodes(parsed), blockNodes(base), false)
	if err != nil {
		return nil, nil, err
	}
	out, err := toList(nodes)
	if err != nil {
		return nil, nil, err
	}
	rep := r.report(out)
	var opaque []RemovedBlock
	rep.Removed, opaque = r.removed(base)
	if len(opaque) > 0 && !allowRemoveOpaque {
		return nil, nil, &RemoveError{Removed: opaque}
	}
	return out, rep, nil
}

// status is what base matching did with a node of the result.
type status uint8

const (
	statusAdded   status = iota // new; the server assigns an id
	statusKept                  // a deep copy of its base node
	statusChanged               // parsed, with its base node's id and hidden fields
)

type reconciler struct {
	h hooks
	// claimOf maps a parsed node with an explicit id to the base node with
	// that id; claimedBy is the inverse.
	claimOf, claimedBy map[blocks.Node]blocks.Node
	// used holds the base nodes the result contains (kept or changed).
	used map[blocks.Node]bool
	// status holds what happened to each node of the result.
	status map[blocks.Node]status
}

// claim checks explicit ids (M-SAFE-06, M-ID-03) and pairs each parsed node
// that has one with the base node of that id, wherever it is.
func (r *reconciler) claim(parsed, base blocks.List) error {
	type explicit struct {
		path string
		n    blocks.Node
	}
	var (
		err  error
		ids  []explicit
		seen = map[string]string{}
	)
	walkBlocks(parsed, func(path string, n blocks.Node) bool {
		if err != nil {
			return false
		}
		if isNil(n) {
			err = &SyntaxError{Reason: "no block at " + path}
			return false
		}
		id := n.NodeID()
		if id == "" {
			if isUnsupported(n) {
				err = &IDError{Path: path, Reason: "unsupported block has no id"}
			}
			return err == nil
		}
		if prev, ok := seen[id]; ok {
			err = &IDError{ID: id, Path: path, Reason: "also used at " + prev}
			return false
		}
		seen[id] = path
		ids = append(ids, explicit{path, n})
		return true
	})
	if err != nil {
		return err
	}
	byID := map[string]blocks.Node{}
	walkBlocks(base, func(_ string, n blocks.Node) bool {
		if isNil(n) {
			return false
		}
		if id := n.NodeID(); id != "" {
			if _, dup := byID[id]; !dup {
				byID[id] = n
			}
		}
		return true
	})
	baseSecret, parsedSecret := underSecret(base), underSecret(parsed)
	for _, e := range ids {
		b, ok := byID[e.n.NodeID()]
		if !ok {
			if isUnsupported(e.n) {
				return &IDError{ID: e.n.NodeID(), Path: e.path, Reason: "unsupported block is not in Base"}
			}
			continue // a new id; the server decides
		}
		if classOf(b) != classOf(e.n) {
			return &IDError{ID: e.n.NodeID(), Path: e.path,
				Reason: fmt.Sprintf("is a %s in Base, not a %s", describe(b), describe(e.n))}
		}
		if isUnsupported(b) && baseSecret[b] && !parsedSecret[e.n] {
			// Its content is invisible here, so neither the editor nor a
			// spoiler guard can tell whether it may be shown.
			return &IDError{ID: e.n.NodeID(), Path: e.path,
				Reason: "unsupported content is inside a secret in Base and can't be moved out of it"}
		}
		r.claimOf[e.n] = b
		r.claimedBy[b] = e.n
		r.used[b] = true
	}
	return nil
}

// underSecret returns the nodes of l that have a secret block among their
// ancestors.
func underSecret(l blocks.List) map[blocks.Node]bool {
	out := map[blocks.Node]bool{}
	var visit func(n blocks.Node, in bool)
	visit = func(n blocks.Node, in bool) {
		if isNil(n) {
			return
		}
		if in {
			out[n] = true
		}
		in = in || n.Kind() == blocks.KindSecret
		if kids, ok := blocks.Children(n); ok {
			for _, c := range kids {
				visit(c, in)
			}
		}
		if ib, ok := n.(*blocks.InfoboxBlock); ok {
			for _, c := range ib.Rows {
				visit(c, in)
			}
		}
	}
	for _, n := range l {
		visit(n, false)
	}
	return out
}

// level reconciles one list of siblings (blocks, or infobox rows when prop
// is set) and returns the result in parsed order.
func (r *reconciler) level(parsed, base []blocks.Node, prop bool) ([]blocks.Node, error) {
	out := make([]blocks.Node, len(parsed))
	var elig []int // parsed nodes left for the diff
	for i, p := range parsed {
		var err error
		switch b, ok := r.claimOf[p]; {
		case ok:
			out[i], err = r.pair(p, b, prop, "", "", false)
		case p.NodeID() != "":
			out[i], err = r.add(p) // an explicit id not in Base
		default:
			elig = append(elig, i)
		}
		if err != nil {
			return nil, err
		}
	}
	var cand []int // base nodes available for the diff
	for j, b := range base {
		if !isNil(b) && !r.used[b] {
			cand = append(cand, j)
		}
	}

	intern := map[string]int{}
	keyOf := func(n blocks.Node) (string, int) {
		k := r.h.key(n)
		c := classOf(n) + "\x00" + k
		v, ok := intern[c]
		if !ok {
			v = len(intern)
			intern[c] = v
		}
		return k, v
	}
	pk, pv := make([]string, len(elig)), make([]int, len(elig))
	for x, i := range elig {
		pk[x], pv[x] = keyOf(parsed[i])
	}
	bk, bv := make([]string, len(cand)), make([]int, len(cand))
	for x, j := range cand {
		bk[x], bv[x] = keyOf(base[j])
	}

	// pm[x] is the cand position elig[x] is matched to, or -1; bm marks
	// matched cand positions.
	pm := make([]int, len(elig))
	for x := range pm {
		pm[x] = -1
	}
	bm := make([]bool, len(cand))
	lcsPairs := lcs(pv, bv)
	for _, m := range lcsPairs {
		pm[m[0]], bm[m[1]] = m[1], true
	}
	// Moves: an unmatched parsed node that renders like a still-unmatched
	// base sibling anywhere in the list is that sibling, moved.
	free := map[int][]int{}
	for y := range cand {
		if !bm[y] {
			free[bv[y]] = append(free[bv[y]], y)
		}
	}
	for x := range elig {
		if pm[x] >= 0 {
			continue
		}
		q := free[pv[x]]
		for k, y := range q {
			if !bm[y] && r.consistent(parsed[elig[x]], base[cand[y]]) {
				pm[x], bm[y] = y, true
				free[pv[x]] = append(q[:k:k], q[k+1:]...)
				break
			}
		}
	}
	for x, y := range pm {
		if y < 0 {
			continue
		}
		p, b := parsed[elig[x]], base[cand[y]]
		var err error
		if r.used[b] { // defensive: never use a base node twice
			out[elig[x]], err = r.add(p)
		} else {
			out[elig[x]], err = r.pair(p, b, prop, pk[x], bk[y], true)
		}
		if err != nil {
			return nil, err
		}
	}
	// Hunks: the unmatched nodes between consecutive LCS matches.
	hunk := func(p0, p1, b0, b1 int) error {
		var ps []blocks.Node
		var at []int
		for x := p0; x < p1; x++ {
			if pm[x] < 0 {
				ps = append(ps, parsed[elig[x]])
				at = append(at, elig[x])
			}
		}
		var bs []blocks.Node
		for y := b0; y < b1; y++ {
			if !bm[y] {
				bs = append(bs, base[cand[y]])
			}
		}
		res, err := r.pairHunk(ps, bs, prop)
		if err != nil {
			return err
		}
		for k, i := range at {
			out[i] = res[k]
		}
		return nil
	}
	pi, bi := 0, 0
	for _, m := range lcsPairs {
		if err := hunk(pi, m[0], bi, m[1]); err != nil {
			return nil, err
		}
		pi, bi = m[0]+1, m[1]+1
	}
	if err := hunk(pi, len(elig), bi, len(cand)); err != nil {
		return nil, err
	}
	return out, nil
}

// pairHunk handles one diff hunk: the i-th parsed node of a kind is paired
// with the i-th base node of that kind and changed; the rest are added.
// Base nodes that render opaque, and parsed raw nodes (which can't take an
// id), are never paired this way.
func (r *reconciler) pairHunk(parsed, base []blocks.Node, prop bool) ([]blocks.Node, error) {
	queue := map[string][]blocks.Node{}
	for _, b := range base {
		if !r.used[b] && !isUnsupported(b) && !r.h.opaque(b) {
			c := classOf(b)
			queue[c] = append(queue[c], b)
		}
	}
	out := make([]blocks.Node, len(parsed))
	for i, p := range parsed {
		c := classOf(p)
		q := queue[c]
		for len(q) > 0 && r.used[q[0]] {
			q = q[1:]
		}
		var err error
		if len(q) > 0 && !isRaw(p) {
			b := q[0]
			q = q[1:]
			out[i], err = r.pair(p, b, prop, "", "", false)
		} else {
			out[i], err = r.add(p)
		}
		queue[c] = q
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// pair returns the result for parsed node p matched with base node b of the
// same class: b verbatim when they render the same, or else p with b's id
// and hidden fields.
func (r *reconciler) pair(p, b blocks.Node, prop bool, pkey, bkey string, haveKeys bool) (blocks.Node, error) {
	if !haveKeys {
		pkey, bkey = r.h.key(p), r.h.key(b)
	}
	if pkey == bkey && r.consistent(p, b) {
		return r.keep(b, prop)
	}
	if isUnsupported(b) {
		return r.changeUnsupported(p, b, prop)
	}
	return r.change(p, b, prop)
}

// keep returns a deep copy of b and marks its subtree used.
func (r *reconciler) keep(b blocks.Node, prop bool) (blocks.Node, error) {
	c, err := copyNode(b, prop)
	if err != nil {
		return nil, err
	}
	r.markUsed(b)
	r.status[c] = statusKept
	return c, nil
}

// changeUnsupported handles an unsupported node whose rendering changed.
// Its content can't be edited, so the result is b; for an infobox row the
// envelope (title, visibility) comes from p.
func (r *reconciler) changeUnsupported(p, b blocks.Node, prop bool) (blocks.Node, error) {
	c, err := copyNode(b, prop)
	if err != nil {
		return nil, err
	}
	r.markUsed(b)
	pp, ok1 := p.(blocks.Property)
	cp, ok2 := c.(blocks.Property)
	if ok1 && ok2 {
		pe, ce := pp.PropertyEnvelope(), cp.PropertyEnvelope()
		if pe != nil && ce != nil {
			ce.Title, ce.IsTitleHidden, ce.IsHidden = pe.Title, pe.IsTitleHidden, pe.IsHidden
			if pe.Rank != "" {
				ce.Rank = pe.Rank
			}
		}
	}
	r.status[c] = statusKept
	if !sameJSON(c, b) {
		r.status[c] = statusChanged
	}
	return c, nil
}

// change gives p the id and hidden fields of b and reconciles their
// children and infobox rows.
func (r *reconciler) change(p, b blocks.Node, prop bool) (blocks.Node, error) {
	r.used[b] = true
	transferHidden(p, b)
	if pc, ok := blocks.Children(p); ok {
		bc, _ := blocks.Children(b)
		res, err := r.level(blockNodes(pc), blockNodes(bc), false)
		if err != nil {
			return nil, err
		}
		l, err := toList(res)
		if err != nil {
			return nil, err
		}
		blocks.SetChildren(p, l)
	}
	if pib, ok := p.(*blocks.InfoboxBlock); ok {
		var brows blocks.Properties
		if bib, ok := b.(*blocks.InfoboxBlock); ok {
			brows = bib.Rows
		}
		res, err := r.level(propNodes(pib.Rows), propNodes(brows), true)
		if err != nil {
			return nil, err
		}
		if pib.Rows, err = toProps(res); err != nil {
			return nil, err
		}
	}
	if sameJSON(p, b) {
		r.markUsed(b)
		r.status[p] = statusKept
	} else {
		r.status[p] = statusChanged
	}
	return p, nil
}

// add keeps p as a new node, still reconciling its descendants so that
// explicit ids inside it (moves into a new container) are honoured.
func (r *reconciler) add(p blocks.Node) (blocks.Node, error) {
	if pc, ok := blocks.Children(p); ok {
		res, err := r.level(blockNodes(pc), nil, false)
		if err != nil {
			return nil, err
		}
		l, err := toList(res)
		if err != nil {
			return nil, err
		}
		blocks.SetChildren(p, l)
	}
	if pib, ok := p.(*blocks.InfoboxBlock); ok {
		res, err := r.level(propNodes(pib.Rows), nil, true)
		if err != nil {
			return nil, err
		}
		if pib.Rows, err = toProps(res); err != nil {
			return nil, err
		}
	}
	r.status[p] = statusAdded
	return p, nil
}

// consistent reports whether replacing parsed subtree p by base subtree b
// keeps every explicit id where the Markdown put it: each explicit id in p
// sits on the parallel node of b, and no node of b is claimed by an
// explicit id elsewhere.
func (r *reconciler) consistent(p, b blocks.Node) bool {
	if isNil(p) || isNil(b) {
		return false
	}
	if id := p.NodeID(); id != "" && id != b.NodeID() {
		return false
	}
	if q, ok := r.claimedBy[b]; ok && q != p {
		return false
	}
	pc, okp := blocks.Children(p)
	bc, okb := blocks.Children(b)
	if okp != okb || len(pc) != len(bc) {
		return false
	}
	for i := range pc {
		if !r.consistent(pc[i], bc[i]) {
			return false
		}
	}
	pib, okp := p.(*blocks.InfoboxBlock)
	bib, okb := b.(*blocks.InfoboxBlock)
	if okp != okb {
		return false
	}
	if okp {
		if len(pib.Rows) != len(bib.Rows) {
			return false
		}
		for i := range pib.Rows {
			if !r.consistent(pib.Rows[i], bib.Rows[i]) {
				return false
			}
		}
	}
	return rowIDsConsistent(p, b)
}

// markUsed marks b and its descendants as used.
func (r *reconciler) markUsed(b blocks.Node) {
	walk(b, "", func(_ string, n blocks.Node) bool {
		if isNil(n) {
			return false
		}
		r.used[n] = true
		return true
	})
}

// report lists, in document order of the result, the kept and changed base
// ids and the paths of the tops of new subtrees.
func (r *reconciler) report(out blocks.List) *Report {
	rep := &Report{}
	var visit func(path string, n blocks.Node, inAdded bool)
	visit = func(path string, n blocks.Node, inAdded bool) {
		switch r.status[n] {
		case statusKept:
			rep.Kept = append(rep.Kept, n.NodeID())
			return
		case statusChanged:
			rep.Changed = append(rep.Changed, n.NodeID())
			inAdded = false
		default:
			if !inAdded {
				rep.Added = append(rep.Added, path)
			}
			inAdded = true
		}
		if l, ok := blocks.Children(n); ok {
			for i, c := range l {
				visit(path+".children["+strconv.Itoa(i)+"]", c, inAdded)
			}
		}
		if ib, ok := n.(*blocks.InfoboxBlock); ok {
			for i, c := range ib.Rows {
				visit(path+".rows["+strconv.Itoa(i)+"]", c, inAdded)
			}
		}
	}
	for i, n := range out {
		visit("blocks["+strconv.Itoa(i)+"]", n, false)
	}
	return rep
}

// removed lists, in Base document order, the tops of the base subtrees the
// result doesn't contain, and separately every opaque node among them.
func (r *reconciler) removed(base blocks.List) (removed, opaque []RemovedBlock) {
	var visit func(n blocks.Node, top int)
	visit = func(n blocks.Node, top int) {
		if isNil(n) {
			return
		}
		if r.used[n] {
			top = -1
		} else {
			if top < 0 {
				removed = append(removed, RemovedBlock{ID: n.NodeID(), Kind: n.Kind()})
				top = len(removed) - 1
			}
			if n.Kind() == blocks.KindSecret {
				removed[top].Secret = true
			}
			if r.h.opaque(n) {
				removed[top].Opaque = true
				opaque = append(opaque, RemovedBlock{ID: n.NodeID(), Kind: n.Kind(), Opaque: true, Secret: r.holdsSecret(n)})
			}
		}
		if l, ok := blocks.Children(n); ok {
			for _, c := range l {
				visit(c, top)
			}
		}
		if ib, ok := n.(*blocks.InfoboxBlock); ok {
			for _, c := range ib.Rows {
				visit(c, top)
			}
		}
	}
	for _, n := range base {
		visit(n, -1)
	}
	return removed, opaque
}

// holdsSecret reports whether the unused part of n's subtree has a secret.
func (r *reconciler) holdsSecret(n blocks.Node) bool {
	found := false
	walk(n, "", func(_ string, c blocks.Node) bool {
		if found || isNil(c) || r.used[c] {
			return false
		}
		found = c.Kind() == blocks.KindSecret
		return !found
	})
	return found
}

// transferHidden gives p the fields of b that Markdown doesn't show: the
// id, a property's rank, and the ids and ranks of rows and items.
func transferHidden(p, b blocks.Node) {
	if p.NodeID() == "" {
		if id := b.NodeID(); id != "" {
			setID(p, id)
		}
	}
	if pp, ok := p.(blocks.Property); ok {
		if bp, ok := b.(blocks.Property); ok {
			pe, be := pp.PropertyEnvelope(), bp.PropertyEnvelope()
			if pe != nil && be != nil && pe.Rank == "" {
				pe.Rank = be.Rank
			}
		}
	}
	if pr, br := kvRows(p), kvRows(b); pr != nil && br != nil {
		matchRows(*pr, *br,
			func(x *blocks.KVRow) *string { return &x.ID },
			func(x *blocks.KVRow) *string { return &x.Rank },
			func(x *blocks.KVRow) string { return x.Key })
	}
	if pi, bi := meterItems(p), meterItems(b); pi != nil && bi != nil {
		matchRows(*pi, *bi,
			func(x *blocks.MeterItem) *string { return &x.ID },
			nil,
			func(x *blocks.MeterItem) string { return x.Name })
	}
	if pi, bi := resourceItems(p), resourceItems(b); pi != nil && bi != nil {
		matchRows(*pi, *bi,
			func(x *blocks.ResourceItem) *string { return &x.ID },
			func(x *blocks.ResourceItem) *string { return &x.Rank },
			resourceKey)
	}
}

// matchRows gives each parsed row the id (and rank, when rank is not nil)
// of a base row: first the base row with its explicit id, else the first
// unused base row with an equal key. No base row is used twice.
func matchRows[T any](p, b []T, id, rank func(*T) *string, key func(*T) string) {
	taken := make([]bool, len(b))
	byID := map[string]int{}
	for j := range b {
		if s := *id(&b[j]); s != "" {
			if _, dup := byID[s]; !dup {
				byID[s] = j
			}
		}
	}
	take := func(i, j int) {
		taken[j] = true
		if *id(&p[i]) == "" {
			*id(&p[i]) = *id(&b[j])
		}
		if rank != nil && *rank(&p[i]) == "" {
			*rank(&p[i]) = *rank(&b[j])
		}
	}
	explicit := make([]bool, len(p))
	for i := range p {
		if s := *id(&p[i]); s != "" {
			explicit[i] = true
			if j, ok := byID[s]; ok && !taken[j] {
				take(i, j)
			}
		}
	}
	byKey := map[string][]int{}
	for j := range b {
		if !taken[j] {
			k := key(&b[j])
			byKey[k] = append(byKey[k], j)
		}
	}
	for i := range p {
		if explicit[i] {
			continue
		}
		k := key(&p[i])
		if q := byKey[k]; len(q) > 0 {
			take(i, q[0])
			byKey[k] = q[1:]
		}
	}
}

// rowIDsConsistent reports whether every explicit row or item id of p is
// on the row of b at the same position.
func rowIDsConsistent(p, b blocks.Node) bool {
	if pr, br := kvRows(p), kvRows(b); pr != nil || br != nil {
		if pr == nil || br == nil || !idsAt(*pr, *br, func(x *blocks.KVRow) string { return x.ID }) {
			return false
		}
	}
	if pi, bi := meterItems(p), meterItems(b); pi != nil || bi != nil {
		if pi == nil || bi == nil || !idsAt(*pi, *bi, func(x *blocks.MeterItem) string { return x.ID }) {
			return false
		}
	}
	if pi, bi := resourceItems(p), resourceItems(b); pi != nil || bi != nil {
		if pi == nil || bi == nil || !idsAt(*pi, *bi, func(x *blocks.ResourceItem) string { return x.ID }) {
			return false
		}
	}
	return true
}

func idsAt[T any](p, b []T, id func(*T) string) bool {
	if len(p) != len(b) {
		return false
	}
	for i := range p {
		if s := id(&p[i]); s != "" && s != id(&b[i]) {
			return false
		}
	}
	return true
}

func kvRows(n blocks.Node) *blocks.KVRows {
	switch v := n.(type) {
	case *blocks.KeyValueBlock:
		return &v.Rows
	case *blocks.KeyValueProperty:
		return &v.Rows
	}
	return nil
}

func meterItems(n blocks.Node) *blocks.MeterItems {
	switch v := n.(type) {
	case *blocks.MeterBlock:
		return &v.Items
	case *blocks.MeterProperty:
		return &v.Items
	}
	return nil
}

func resourceItems(n blocks.Node) *blocks.ResourceItems {
	switch v := n.(type) {
	case *blocks.ResourceCollectionBlock:
		return &v.Items
	case *blocks.ResourceCollectionProperty:
		return &v.Items
	case *blocks.ResourceLinkBlock:
		return &v.Items
	case *blocks.ResourceLinkProperty:
		return &v.Items
	}
	return nil
}

func resourceKey(x *blocks.ResourceItem) string {
	if d, ok := x.DocumentID.Get(); ok {
		return x.ResourceID + "\x00" + d
	}
	return x.ResourceID
}

func setID(n blocks.Node, id string) {
	switch v := n.(type) {
	case blocks.Block:
		blocks.WithID(id, v)
	case blocks.Property:
		if e := v.PropertyEnvelope(); e != nil {
			e.ID = id
		}
	}
}

// copyNode deep-copies n through its JSON. The copy encodes to exactly the
// bytes n does (M-ID-01); if decoding would change them, the copy is a raw
// node holding those bytes.
func copyNode(n blocks.Node, prop bool) (blocks.Node, error) {
	data, err := json.Marshal(n)
	if err != nil {
		return nil, fmt.Errorf("markdown: base %s %q does not encode: %w", n.Kind(), n.NodeID(), err)
	}
	var c blocks.Node
	if prop {
		p, err := blocks.UnmarshalProperty(data)
		if err != nil {
			return nil, fmt.Errorf("markdown: base %s %q: %w", n.Kind(), n.NodeID(), err)
		}
		if again, err := json.Marshal(p); err != nil || !bytes.Equal(again, data) {
			p = &blocks.RawProperty{KindName: n.Kind(), JSON: data}
		}
		c = p
	} else {
		b, err := blocks.Unmarshal(data)
		if err != nil {
			return nil, fmt.Errorf("markdown: base %s %q: %w", n.Kind(), n.NodeID(), err)
		}
		if again, err := json.Marshal(b); err != nil || !bytes.Equal(again, data) {
			b = &blocks.RawBlock{KindName: n.Kind(), JSON: data}
		}
		c = b
	}
	return c, nil
}

func sameJSON(a, b blocks.Node) bool {
	x, err := json.Marshal(a)
	if err != nil {
		return false
	}
	y, err := json.Marshal(b)
	return err == nil && bytes.Equal(x, y)
}

// walk visits n and its descendants in pre-order with Walk-style paths. It
// calls visit for nil nodes too, and descends only when visit returns true.
func walk(n blocks.Node, path string, visit func(string, blocks.Node) bool) {
	if !visit(path, n) || isNil(n) {
		return
	}
	if l, ok := blocks.Children(n); ok {
		for i, c := range l {
			walk(c, path+".children["+strconv.Itoa(i)+"]", visit)
		}
	}
	if ib, ok := n.(*blocks.InfoboxBlock); ok {
		for i, c := range ib.Rows {
			walk(c, path+".rows["+strconv.Itoa(i)+"]", visit)
		}
	}
}

func walkBlocks(l blocks.List, visit func(string, blocks.Node) bool) {
	for i, b := range l {
		walk(b, "blocks["+strconv.Itoa(i)+"]", visit)
	}
}

// isNil reports whether n is nil or a nil pointer.
func isNil(n blocks.Node) bool {
	if n == nil {
		return true
	}
	v := reflect.ValueOf(n)
	return v.Kind() == reflect.Pointer && v.IsNil()
}

func isUnsupported(n blocks.Node) bool {
	switch n.(type) {
	case *blocks.UnsupportedBlock, *blocks.UnsupportedProperty:
		return true
	}
	return false
}

func isRaw(n blocks.Node) bool {
	switch n.(type) {
	case *blocks.RawBlock, *blocks.RawProperty:
		return true
	}
	return false
}

// classOf is a node's kind plus whether it is a block or an infobox row: a
// keyValue block and a keyValue row never match.
func classOf(n blocks.Node) string {
	if _, ok := n.(blocks.Property); ok {
		return "row:" + string(n.Kind())
	}
	return "block:" + string(n.Kind())
}

func describe(n blocks.Node) string {
	if _, ok := n.(blocks.Property); ok {
		return string(n.Kind()) + " infobox row"
	}
	return string(n.Kind()) + " block"
}

func blockNodes(l blocks.List) []blocks.Node {
	out := make([]blocks.Node, len(l))
	for i, b := range l {
		if b != nil {
			out[i] = b
		}
	}
	return out
}

func propNodes(ps blocks.Properties) []blocks.Node {
	out := make([]blocks.Node, len(ps))
	for i, p := range ps {
		if p != nil {
			out[i] = p
		}
	}
	return out
}

func toList(ns []blocks.Node) (blocks.List, error) {
	out := make(blocks.List, len(ns))
	for i, n := range ns {
		b, ok := n.(blocks.Block)
		if !ok || isNil(n) {
			return nil, fmt.Errorf("markdown: internal error: %T in a block list", n)
		}
		out[i] = b
	}
	return out, nil
}

func toProps(ns []blocks.Node) (blocks.Properties, error) {
	out := make(blocks.Properties, len(ns))
	for i, n := range ns {
		p, ok := n.(blocks.Property)
		if !ok || isNil(n) {
			return nil, fmt.Errorf("markdown: internal error: %T in infobox rows", n)
		}
		out[i] = p
	}
	return out, nil
}
