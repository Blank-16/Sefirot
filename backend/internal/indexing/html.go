package indexing

import (
	"bytes"
	nethtml "golang.org/x/net/html"
	"sefirot/internal/database"
	"sort"
	"strings"
)

type DocumentMetadata struct {
	Description string
	Summary     string
	DocType     string
	Tags        []string
	Aliases     []string
	Headings    []database.DocumentHeading
}

func visibleNodeText(n *nethtml.Node) string {
	var b strings.Builder
	var walk func(*nethtml.Node)
	walk = func(node *nethtml.Node) {
		if node.Type == nethtml.ElementNode {
			tag := strings.ToLower(node.Data)
			if tag == "script" || tag == "style" || tag == "noscript" || tag == "template" {
				return
			}
		}
		if node.Type == nethtml.TextNode {
			b.WriteString(node.Data)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return b.String()
}

func ExtractHTML(raw []byte) (title, plain string, links []string, err error) {
	title, plain, links, _, _, err = ExtractHTMLDetails(raw)
	return
}

func ExtractHTMLDetails(raw []byte) (title, plain string, links []string, relationships []database.DocumentRelationship, metadata DocumentMetadata, err error) {
	root, err := nethtml.Parse(bytes.NewReader(raw))
	if err != nil {
		return "", "", nil, nil, metadata, err
	}
	var textParts []string
	linkSet := map[string]struct{}{}
	relSet := map[string]database.DocumentRelationship{}
	tagSet := map[string]struct{}{}
	var titleText, h1Text strings.Builder
	var inTitle, inH1 bool
	walk := func(n *nethtml.Node) {}
	walk = func(n *nethtml.Node) {
		if n.Type == nethtml.ElementNode {
			tag := strings.ToLower(n.Data)
			if tag == "script" || tag == "style" || tag == "noscript" || tag == "template" {
				return
			}
			if tag == "title" {
				inTitle = true
			}
			if tag == "h1" && h1Text.Len() == 0 {
				inH1 = true
			}
			if len(tag) == 2 && tag[0] == 'h' && tag[1] >= '1' && tag[1] <= '6' {
				if heading := strings.TrimSpace(visibleNodeText(n)); heading != "" && len(metadata.Headings) < 200 {
					metadata.Headings = append(metadata.Headings, database.DocumentHeading{Level: int(tag[1] - '0'), Text: heading})
				}
			}
			if tag == "meta" {
				attrs := map[string]string{}
				for _, a := range n.Attr {
					attrs[strings.ToLower(a.Key)] = strings.TrimSpace(a.Val)
				}
				name := strings.ToLower(attrs["name"])
				if name == "description" || name == "vault:summary" {
					metadata.Description = attrs["content"]
				}
				if name == "vault:aliases" {
					for _, v := range strings.FieldsFunc(attrs["content"], func(r rune) bool { return r == ',' || r == ';' }) {
						if v = strings.TrimSpace(v); v != "" {
							metadata.Aliases = append(metadata.Aliases, v)
						}
					}
				}
				if name == "vault:type" {
					metadata.DocType = attrs["content"]
				}
				if name == "keywords" || name == "vault:tags" {
					for _, v := range strings.FieldsFunc(attrs["content"], func(r rune) bool { return r == ',' || r == ';' }) {
						v = strings.TrimSpace(v)
						if v != "" {
							tagSet[strings.ToLower(v)] = struct{}{}
						}
					}
				}
				if strings.EqualFold(attrs["property"], "og:description") && metadata.Description == "" {
					metadata.Description = attrs["content"]
				}
			}
			if tag == "a" {
				attrs := map[string]string{}
				for _, a := range n.Attr {
					attrs[strings.ToLower(a.Key)] = strings.TrimSpace(a.Val)
				}
				if target := attrs["data-wikilink"]; target != "" {
					linkSet[target] = struct{}{}
					relType := strings.ToLower(strings.TrimSpace(attrs["data-relation"]))
					switch relType {
					case "related-to", "depends-on", "implements", "extends", "contradicts", "references", "parent-of":
					default:
						relType = "related-to"
					}
					relSet[target+"\x00"+relType] = database.DocumentRelationship{Target: target, Type: relType}
				}
			}
		}
		if n.Type == nethtml.TextNode {
			t := strings.TrimSpace(n.Data)
			if t != "" {
				textParts = append(textParts, t)
				if inTitle {
					titleText.WriteString(t)
				}
				if inH1 {
					h1Text.WriteString(t)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == nethtml.ElementNode {
			if strings.EqualFold(n.Data, "title") {
				inTitle = false
			}
			if strings.EqualFold(n.Data, "h1") {
				inH1 = false
			}
		}
	}
	walk(root)
	title = strings.TrimSpace(titleText.String())
	if title == "" {
		title = strings.TrimSpace(h1Text.String())
	}
	if title == "" {
		title = "Untitled"
	}
	links = make([]string, 0, len(linkSet))
	for s := range linkSet {
		links = append(links, s)
	}
	sort.Strings(links)
	relationships = make([]database.DocumentRelationship, 0, len(relSet))
	for _, r := range relSet {
		relationships = append(relationships, r)
	}
	sort.Slice(relationships, func(i, j int) bool {
		if relationships[i].Target == relationships[j].Target {
			return relationships[i].Type < relationships[j].Type
		}
		return relationships[i].Target < relationships[j].Target
	})
	metadata.Tags = make([]string, 0, len(tagSet))
	for tag := range tagSet {
		metadata.Tags = append(metadata.Tags, tag)
	}
	sort.Strings(metadata.Tags)
	if metadata.DocType == "" {
		metadata.DocType = "note"
	}
	plain = strings.Join(textParts, " ")
	if metadata.Summary == "" {
		metadata.Summary = metadata.Description
	}
	return title, plain, links, relationships, metadata, nil
}
