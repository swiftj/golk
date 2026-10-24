package blocks

// Children returns the children of a container block (or a text field
// property). ok is false for a node that has no children list.
func Children(n Node) (l List, ok bool) {
	c, ok := n.(container)
	if !ok {
		return nil, false
	}
	return *c.childList(), true
}

// SetChildren replaces the children of a container block (or a text field
// property). It reports false, and changes nothing, for a node that has no
// children list.
func SetChildren(n Node, l List) bool {
	c, ok := n.(container)
	if !ok {
		return false
	}
	*c.childList() = l
	return true
}

// HasUnmodelled reports whether v carries fields the server sent that this
// package does not model (DECISIONS.md D-008, A-32). It looks at v itself,
// not at its children, rows or items. v is a block, property, run, row,
// item, viewport or filter; anything else reports false.
func HasUnmodelled(v any) bool {
	h, ok := v.(interface{ hasExtras() bool })
	return ok && h.hasExtras()
}

func (e extras) hasExtras() bool { return len(e.m) > 0 }

func (r *Mention) hasExtras() bool { return len(r.m) > 0 || len(r.refExtra) > 0 }
