package indexing

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"go.abhg.dev/goldmark/wikilink"
	"html"
	"io"
	"path"
	"regexp"
	"strings"
)

const sourceCommentPrefix = "<!-- vault-source-md:"

var sourceCommentRE = regexp.MustCompile(`(?s)<!--\s*vault-source-md:([A-Za-z0-9+/=]*)\s*-->`)

type wikiResolver struct{}

type wikiNodeRenderer struct{ resolver wikilink.Resolver }

func markdownEngine() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithInlineParsers(util.Prioritized(&wikilink.Parser{}, 199), util.Prioritized(mathParser{}, 150))),
		goldmark.WithRendererOptions(gmhtml.WithUnsafe(), renderer.WithNodeRenderers(util.Prioritized(&wikiNodeRenderer{resolver: wikiResolver{}}, 199), util.Prioritized(mathRenderer{}, 200))),
	)
}

func metaTag(name, content string) string {
	return `<meta name="` + name + `" content="` + html.EscapeString(content) + `">`
}

func RenderMarkdown(source string) (string, []string, error) {
	md := markdownEngine()
	front, content := ParseFrontMatter(source)
	var body bytes.Buffer
	if err := md.Convert([]byte(content), &body); err != nil {
		return "", nil, err
	}
	links := extractMarkdownLinks([]byte(content))
	bodyHTML := body.String()
	title, _, _, _, _, _ := ExtractHTMLDetails([]byte(bodyHTML))
	if t := front.first("title"); t != "" {
		title = t
	}
	var metas string
	if v := front.first("type"); v != "" {
		metas += metaTag("vault:type", v)
	}
	if v := front.first("description"); v != "" {
		metas += metaTag("description", v)
	}
	if v := front.list("tags"); len(v) > 0 {
		metas += metaTag("vault:tags", strings.Join(v, ", "))
	}
	if v := front.list("aliases"); len(v) > 0 {
		metas += metaTag("vault:aliases", strings.Join(v, ", "))
	}
	doc := `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="light dark"><title>` + html.EscapeString(title) + `</title>` + metas + `<style>body{max-width:820px;margin:2.5rem auto;padding:0 1.5rem 4rem;font:16px/1.75 system-ui,sans-serif;color:CanvasText;background:Canvas}h1,h2,h3{line-height:1.25}img{max-width:100%;height:auto}pre{overflow:auto;padding:1rem;border-radius:.5rem;background:color-mix(in srgb,CanvasText 8%,Canvas)}a{color:LinkText}table{border-collapse:collapse}td,th{border:1px solid color-mix(in srgb,CanvasText 25%,Canvas);padding:.45rem}</style></head><body>` + bodyHTML + `</body></html>`
	encoded := base64.StdEncoding.EncodeToString([]byte(source))
	doc += "\n" + sourceCommentPrefix + encoded + " -->\n"
	return doc, links, nil
}

func extractMarkdownLinks(source []byte) []string {
	md := markdownEngine()
	doc := md.Parser().Parse(text.NewReader(source))
	set := map[string]struct{}{}
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if w, ok := n.(*wikilink.Node); ok && !w.Embed {
				target := strings.TrimSpace(string(w.Target))
				if target != "" {
					set[target] = struct{}{}
				}
			}
		}
		return ast.WalkContinue, nil
	})
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	return out
}

func ExtractMarkdownSource(raw []byte) (string, bool) {
	m := sourceCommentRE.FindSubmatch(raw)
	if len(m) != 2 {
		return string(raw), false
	}
	decoded, err := base64.StdEncoding.DecodeString(string(m[1]))
	if err != nil {
		return string(raw), false
	}
	return string(decoded), true
}

func StripSourceComment(raw []byte) []byte { return sourceCommentRE.ReplaceAll(raw, nil) }

func (wikiResolver) ResolveWikilink(n *wikilink.Node) ([]byte, error) {
	target := strings.TrimSpace(string(n.Target))
	if target == "" {
		return []byte("#"), nil
	}
	if strings.HasPrefix(target, "#") {
		return []byte(target), nil
	}
	if path.Ext(target) == "" {
		target += ".html"
	}
	return []byte("#"), nil
}

func (r *wikiNodeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(wikilink.Kind, r.render)
}

func (r *wikiNodeRenderer) render(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*wikilink.Node)
	if !entering {
		if n.Embed {
			return ast.WalkContinue, nil
		}
		_, err := io.WriteString(w, `</a>`)
		return ast.WalkContinue, err
	}
	target := strings.TrimSpace(string(n.Target))
	if n.Embed {
		_, _ = r.resolver.ResolveWikilink(n)
		_, err := fmt.Fprintf(w, `<img src="/vault-assets/%s" alt="%s" loading="lazy">`, html.EscapeString(target), html.EscapeString(target))
		return ast.WalkSkipChildren, err
	}
	_, _ = r.resolver.ResolveWikilink(n)
	_, err := fmt.Fprintf(w, `<a href="#" data-wikilink="%s" class="wiki-link">`, html.EscapeString(target))
	return ast.WalkContinue, err
}
