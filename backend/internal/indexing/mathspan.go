package indexing

import (
	"bytes"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Markdown treats _, *, \ and brackets as syntax, which silently corrupts LaTeX.
// mathParser recognises math spans and keeps their source byte-for-byte so a
// MathJax pass in the browser sees exactly what the author typed.

var kindMath = ast.NewNodeKind("MathSpan")

type mathNode struct {
	ast.BaseInline
	raw []byte
}

func (n *mathNode) Kind() ast.NodeKind            { return kindMath }
func (n *mathNode) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

const maxMathLines = 200

type mathParser struct{}

func (mathParser) Trigger() []byte { return []byte{'$', '\\'} }

func (mathParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, seg := block.PeekLine()
	if len(line) < 2 {
		return nil
	}
	switch {
	case line[0] == '\\' && line[1] == '$':
		block.Advance(2)
		return &mathNode{raw: []byte(`\$`)}
	case line[0] == '\\' && line[1] == '(':
		if end := bytes.Index(line[2:], []byte(`\)`)); end > 0 {
			n := end + 4
			block.Advance(n)
			return &mathNode{raw: append([]byte(nil), line[:n]...)}
		}
		return nil
	case line[0] == '\\' && line[1] == '[':
		return parseDisplay(block, `\[`, `\]`)
	case line[0] == '$' && line[1] == '$':
		return parseDisplay(block, "$$", "$$")
	case line[0] == '$':
		return parseDollarInline(block, line, seg)
	}
	return nil
}

func parseDollarInline(block text.Reader, line []byte, seg text.Segment) ast.Node {
	if len(line) < 3 || line[1] == ' ' || line[1] == '\t' || line[1] == '\n' || line[1] == '\r' {
		return nil
	}
	for i := 2; i < len(line); i++ {
		switch line[i] {
		case '\\':
			i++
		case '$':
			prev := line[i-1]
			if prev == ' ' || prev == '\t' {
				continue
			}
			if i+1 < len(line) && line[i+1] >= '0' && line[i+1] <= '9' {
				continue
			}
			block.Advance(i + 1)
			return &mathNode{raw: append([]byte(nil), line[:i+1]...)}
		}
	}
	return nil
}

func parseDisplay(block text.Reader, open, closeDelim string) ast.Node {
	startLine, startPos := block.Position()
	var raw []byte
	first := true
	for n := 0; n < maxMathLines; n++ {
		line, _ := block.PeekLine()
		if line == nil {
			break
		}
		from := 0
		if first {
			from = len(open)
		}
		if end := bytes.Index(line[from:], []byte(closeDelim)); end >= 0 {
			total := from + end + len(closeDelim)
			raw = append(raw, line[:total]...)
			block.Advance(total)
			return &mathNode{raw: raw}
		}
		raw = append(raw, line...)
		first = false
		if _, _, ok := nextLine(block); !ok {
			break
		}
	}
	block.SetPosition(startLine, startPos)
	return nil
}

func nextLine(block text.Reader) (int, text.Segment, bool) {
	line, seg := block.PeekLine()
	if line == nil {
		return 0, seg, false
	}
	block.Advance(len(line))
	l, _ := block.Position()
	return l, seg, true
}

type mathRenderer struct{}

func (mathRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindMath, func(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			_, _ = w.Write(util.EscapeHTML(node.(*mathNode).raw))
		}
		return ast.WalkContinue, nil
	})
}
