package blocks

import (
	"strconv"
	"strings"
)

// Walk calls visit for every block of l and its descendants in pre-order,
// with the block's dotted path: blocks[0], blocks[0].children[1], and
// blocks[2].rows[0] for an infobox row (a Property). These are the paths the
// server reports in errors[].path. Walk stops when visit returns false.
func Walk(l List, visit func(path string, n Node) bool) {
	walkList(l, "blocks", visit)
}

func walkList(l List, prefix string, visit func(string, Node) bool) bool {
	for i, b := range l {
		if !walkNode(b, prefix+"["+strconv.Itoa(i)+"]", visit) {
			return false
		}
	}
	return true
}

func walkNode(n Node, path string, visit func(string, Node) bool) bool {
	if !visit(path, n) {
		return false
	}
	if c, ok := n.(container); ok {
		if !walkList(*c.childList(), path+".children", visit) {
			return false
		}
	}
	if ib, ok := n.(*InfoboxBlock); ok {
		for k, p := range ib.Rows {
			if !walkNode(p, path+".rows["+strconv.Itoa(k)+"]", visit) {
				return false
			}
		}
	}
	return true
}

// FindBlock returns the first node in Walk order whose id is id, or nil.
func FindBlock(l List, id string) Node {
	var found Node
	Walk(l, func(_ string, n Node) bool {
		if n.NodeID() == id {
			found = n
			return false
		}
		return true
	})
	return found
}

// PlainText returns the text of l the way the server derives
// DocumentBlocks.text: one line per text-bearing block, joined with "\n".
// Paragraphs, headings and task items contribute their text (a line break
// is "\n", a mention its label or else its resource's name) and code blocks
// their code; empty lines are left out. Containers contribute their
// children. Rules, key-value rows, infoboxes, meters, links, collections,
// images, embeds, maps, block titles, expand titles and unsupported or
// unknown blocks contribute nothing (DECISIONS.md D-009, A-17).
func PlainText(l List) string {
	var lines []string
	collectText(l, &lines)
	return strings.Join(lines, "\n")
}

func collectText(l List, lines *[]string) {
	add := func(s string) {
		if s != "" {
			*lines = append(*lines, s)
		}
	}
	for _, b := range l {
		switch v := b.(type) {
		case *ParagraphBlock:
			add(RunsText(v.Content))
		case *HeadingBlock:
			add(RunsText(v.Content))
		case *TaskItemBlock:
			add(RunsText(v.Content))
		case *CodeBlock:
			add(v.Text)
		case *InfoboxBlock:
		case container:
			collectText(*v.childList(), lines)
		}
	}
}

// RunsText is the plain text of runs: text as is, a line break as "\n", and
// a mention as its label or else its resource's name.
func RunsText(rs Runs) string {
	var b strings.Builder
	for _, r := range rs {
		switch v := r.(type) {
		case *Text:
			b.WriteString(v.Text)
		case *Break:
			b.WriteByte('\n')
		case *Mention:
			if l, ok := v.Label.Get(); ok {
				b.WriteString(l)
			} else {
				b.WriteString(v.Name.Or(""))
			}
		}
	}
	return b.String()
}
