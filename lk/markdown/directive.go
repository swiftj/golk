package markdown

import (
	"fmt"
	"strings"
	"sync"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// The goldmark parser: CommonMark, the GFM table, strikethrough and task
// list extensions, and the LK Markdown block and inline parsers. Every block
// parser is wrapped in a depth guard, so deeply nested input never makes
// goldmark's per-line work grow past maxOpen open blocks (M-SAFE-07).
//
// The parser holds no state between calls (all state lives in the
// parser.Context), so one instance is shared.
var gm = sync.OnceValue(func() parser.Parser {
	var bps []util.PrioritizedValue
	for _, v := range parser.DefaultBlockParsers() {
		bp := v.Value.(parser.BlockParser)
		bps = append(bps, util.Prioritized(&guarded{inner: bp, fence: isFencedCodeParser(bp)}, v.Priority))
	}
	bps = append(bps,
		util.Prioritized(&guarded{inner: directiveParser{}}, 650),
		util.Prioritized(&guarded{inner: attrLineParser{}}, 660),
	)
	return parser.NewParser(
		parser.WithBlockParsers(bps...),
		parser.WithInlineParsers(append(parser.DefaultInlineParsers(),
			util.Prioritized(extension.NewTaskCheckBoxParser(), 0),
			util.Prioritized(spanParser{}, 150),
			util.Prioritized(attrSetParser{}, 160),
			util.Prioritized(extension.NewStrikethroughParser(), 500),
		)...),
		parser.WithParagraphTransformers(
			util.Prioritized(refGuard{}, 100),
			util.Prioritized(extension.NewTableParagraphTransformer(), 200),
		),
		parser.WithASTTransformers(util.Prioritized(extension.NewTableASTTransformer(), 0)),
	)
})

func isFencedCodeParser(bp parser.BlockParser) bool {
	t := bp.Trigger()
	return len(t) == 2 && t[0] == '~' && t[1] == '`'
}

// maxOpen bounds goldmark's open blocks. The exact limit of 64 block levels
// is checked on the converted blocks; this only stops pathological input
// early.
const maxOpen = 72

var (
	closedAttr  = []byte("lkClosed")
	depthErrKey = parser.NewContextKey()
)

// guarded wraps a block parser with the depth guard, and records on a
// fenced code block whether its closing fence was seen.
type guarded struct {
	inner parser.BlockParser
	fence bool
}

func (g *guarded) Trigger() []byte { return g.inner.Trigger() }

func (g *guarded) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	// A list just opened must get its item (goldmark's list parser needs
	// one), so the guard never refuses under a list; the list itself was
	// guarded.
	if _, isList := parent.(*ast.List); !isList && len(pc.OpenedBlocks()) >= maxOpen {
		// Refuse to open and fail the parse afterwards.
		if pc.Get(depthErrKey) == nil {
			_, seg := reader.PeekLine()
			pc.Set(depthErrKey, seg.Start)
		}
		return nil, parser.NoChildren
	}
	return g.inner.Open(parent, reader, pc)
}

func (g *guarded) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	st := g.inner.Continue(node, reader, pc)
	if g.fence && st&parser.Close != 0 {
		node.SetAttribute(closedAttr, true)
	}
	return st
}

func (g *guarded) Close(node ast.Node, reader text.Reader, pc parser.Context) {
	g.inner.Close(node, reader, pc)
	if _, ok := node.(*ast.Heading); ok {
		markGuard(node, reader.Source(), pc)
	}
}

func (g *guarded) CanInterruptParagraph() bool { return g.inner.CanInterruptParagraph() }
func (g *guarded) CanAcceptIndentedLine() bool { return g.inner.CanAcceptIndentedLine() }

// fenceClosed reports whether a fenced code block saw its closing fence.
func fenceClosed(n ast.Node) bool {
	_, ok := n.Attribute(closedAttr)
	return ok
}

// directiveNode is a ::: container directive or a :: leaf directive.
type directiveNode struct {
	ast.BaseBlock
	name   string
	leaf   bool
	attrs  []attr
	bad    string // why the fence line is malformed
	closed bool   // the closing fence was seen
}

var kindDirective = ast.NewNodeKind("LKDirective")

func (n *directiveNode) Kind() ast.NodeKind { return kindDirective }
func (n *directiveNode) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, map[string]string{"Name": n.name, "Bad": n.bad}, nil)
}

type directiveParser struct{}

func (directiveParser) Trigger() []byte { return []byte{':'} }

func (directiveParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, seg := reader.PeekLine()
	pos := pc.BlockOffset()
	if pos < 0 {
		return nil, parser.NoChildren
	}
	rest := line[pos:]
	colons := 0
	for colons < len(rest) && rest[colons] == ':' {
		colons++
	}
	leaf := colons == 2
	if colons < 2 || leaf && (colons == len(rest) || !isNameStart(rest[colons])) {
		return nil, parser.NoChildren
	}
	n := &directiveNode{leaf: leaf}
	n.SetPos(seg.Start + pos - seg.Padding)
	reader.AdvanceToEOL()
	body := []byte(strings.TrimRight(string(rest[colons:]), " \t\r\n"))
	j := 0
	for j < len(body) && isNameChar(body[j]) {
		j++
	}
	switch {
	case len(body) == 0:
		n.leaf, n.bad = true, "closing ::: with no open directive"
		return n, parser.NoChildren
	case j == 0 || !isNameStart(body[0]):
		n.bad = "malformed directive fence: the name must follow the colons"
	case j < len(body) && body[j] != ' ' && body[j] != '\t':
		n.bad = fmt.Sprintf("malformed directive name %q", body)
	default:
		n.name = string(body[:j])
		var reason string
		n.attrs, _, reason = scanAttrs(body, j, false, false)
		if reason != "" {
			n.bad = reason
		}
	}
	if n.leaf {
		return n, parser.NoChildren
	}
	return n, parser.HasChildren
}

func (directiveParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	n := node.(*directiveNode)
	if n.leaf {
		return parser.Close
	}
	line, _ := reader.PeekLine()
	if w, pos := util.IndentWidth(line, reader.LineOffset()); w < 4 && isClosingFence(line[pos:]) && !deeperOwnsLine(n, pc) {
		reader.AdvanceToEOL()
		n.closed = true
		return parser.Close
	}
	return parser.Continue | parser.HasChildren
}

func (directiveParser) Close(ast.Node, text.Reader, parser.Context) {}
func (directiveParser) CanInterruptParagraph() bool                 { return true }
func (directiveParser) CanAcceptIndentedLine() bool                 { return false }

// isClosingFence reports whether line is three or more colons and nothing
// else.
func isClosingFence(line []byte) bool {
	i := 0
	for i < len(line) && line[i] == ':' {
		i++
	}
	return i >= 3 && util.IsBlank(line[i:])
}

// deeperOwnsLine reports whether a block opened inside n owns a closing
// fence line: a nested directive (the closing fence closes the innermost
// one) or a fenced code block (the line is code).
func deeperOwnsLine(n *directiveNode, pc parser.Context) bool {
	opened := pc.OpenedBlocks()
	for i, b := range opened {
		if b.Node != n {
			continue
		}
		for _, d := range opened[i+1:] {
			switch v := d.Node.(type) {
			case *directiveNode:
				if !v.leaf {
					return true
				}
			case *ast.FencedCodeBlock:
				return true
			}
		}
		return false
	}
	return false
}

// attrLineNode is a line holding only an attribute set, which applies to
// the block just above it (M-DIR-04).
type attrLineNode struct {
	ast.BaseBlock
	attrs []attr
}

var kindAttrLine = ast.NewNodeKind("LKAttrLine")

func (n *attrLineNode) Kind() ast.NodeKind { return kindAttrLine }
func (n *attrLineNode) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, nil, nil)
}

type attrLineParser struct{}

func (attrLineParser) Trigger() []byte { return []byte{'{'} }

func (attrLineParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, seg := reader.PeekLine()
	pos := pc.BlockOffset()
	if pos < 0 {
		return nil, parser.NoChildren
	}
	rest := util.TrimRightSpace(line[pos:])
	attrs, n, ok := scanAttrSet(rest)
	if !ok || n != len(rest) {
		return nil, parser.NoChildren
	}
	node := &attrLineNode{attrs: attrs}
	node.SetPos(seg.Start + pos - seg.Padding)
	reader.AdvanceToEOL()
	return node, parser.NoChildren
}

func (attrLineParser) Continue(ast.Node, text.Reader, parser.Context) parser.State {
	return parser.Close
}
func (attrLineParser) Close(ast.Node, text.Reader, parser.Context) {}
func (attrLineParser) CanInterruptParagraph() bool                 { return true }
func (attrLineParser) CanAcceptIndentedLine() bool                 { return false }

// refGuard runs the link reference definition transformer and records any
// definition it finds, which LK Markdown rejects (M-SAFE-03).
type refGuard struct{}

var (
	refErrKey  = parser.NewContextKey()
	markErrKey = parser.NewContextKey()
)

// maxMarks bounds the unescaped '[', '*', '_' and '~' in one paragraph or
// heading. goldmark's link and emphasis parsing is quadratic in them
// ("[a](" or "*a_" repeated takes minutes at 1 MiB); with the bound the
// worst case is a few seconds (M-API-03, M-SAFE-07). The renderer writes a
// block over the bound as lk-raw, which is not inline-parsed.
const maxMarks = 2000

// markGuard records an error and empties n's lines, so inline parsing
// skips it, when n holds more than maxMarks markers. It reports whether it
// did.
func markGuard(n ast.Node, src []byte, pc parser.Context) bool {
	lines := n.Lines()
	count := 0
	for i := range lines.Len() {
		s := lines.At(i)
		v := s.Value(src)
		for j := 0; j < len(v); j++ {
			switch v[j] {
			case '\\':
				j++
			case '[', '*', '_', '~':
				count++
			}
		}
	}
	if count <= maxMarks {
		return false
	}
	if pc.Get(markErrKey) == nil {
		pc.Set(markErrKey, lines.At(0).Start)
	}
	n.SetLines(text.NewSegments())
	return true
}

func (refGuard) Transform(node *ast.Paragraph, reader text.Reader, pc parser.Context) {
	if markGuard(node, reader.Source(), pc) {
		return
	}
	pos := -1
	if node.Lines().Len() > 0 {
		pos = node.Lines().At(0).Start
	}
	before := len(pc.References())
	parser.LinkReferenceParagraphTransformer.Transform(node, reader, pc)
	if len(pc.References()) != before && pc.Get(refErrKey) == nil {
		pc.Set(refErrKey, pos)
	}
}
