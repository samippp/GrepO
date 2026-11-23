// diagram.go: --format mermaid|dot, a --depth tree drawn as a graph of
// functions (docs/output.md, Mermaid and DOT).

package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/samippp/grepo/internal/graph"
	"github.com/samippp/grepo/internal/query"
)

// mermaidNodes is about where Mermaid stops rendering legibly.
const mermaidNodes = 100

type nodeKind int

const (
	funcNode    nodeKind = iota // a function, or package-level code
	ifaceNode                   // an interface method
	dynamicNode                 // a func value nothing resolves
)

type dnode struct {
	id, label string
	kind      nodeKind
	outside   bool // outside the repo
}

type dedge struct {
	from, to int
	label    string
	dotted   bool // an interface method to an implementation
}

// diagram is a node per function, in the order first reached, and an edge
// per caller, callee and label. The root is node 0.
type diagram struct {
	nodes []dnode
	index map[string]int // by id
	edges []dedge
	seen  map[dedge]bool
}

func (d *diagram) node(id, label string, kind nodeKind, outside bool) int {
	if i, ok := d.index[id]; ok {
		return i
	}
	d.index[id] = len(d.nodes)
	d.nodes = append(d.nodes, dnode{id: id, label: label, kind: kind, outside: outside})
	return len(d.nodes) - 1
}

func (d *diagram) edge(e dedge) {
	if !d.seen[e] {
		d.seen[e] = true
		d.edges = append(d.edges, e)
	}
}

// diagramBuilder adds nodes named as text names them.
type diagramBuilder struct {
	*diagram
	p *printer
}

func newDiagramBuilder(p *printer) diagramBuilder {
	return diagramBuilder{&diagram{index: map[string]int{}, seen: map[dedge]bool{}}, p}
}

func (b diagramBuilder) fn(id string, kind nodeKind) int {
	ix := b.p.rg.ix
	label := ix.Short(id)
	if ix.IsPackage(id) {
		label += " (package level)"
	}
	f := ix.Function(id)
	return b.node(id, label, kind, f != nil && f.Pos == "")
}

// dynamic is the field or var a call goes through, or a func value per caller.
func (b diagramBuilder) dynamic(src int, c *graph.FunctionCall) int {
	if c.Through != "" {
		return b.node(c.Through, b.p.rg.ix.Short(c.Through), dynamicNode, false)
	}
	return b.node(b.nodes[src].id+" (func value)", "(func value)", dynamicNode, false)
}

// label is an edge's tag: "" for a direct call.
func (b diagramBuilder) label(s query.Site) string {
	switch s.Kind {
	case query.Local:
		return "through a local"
	case query.Value:
		return b.p.valueTag(s)
	case query.Dynamic:
		return "dynamic"
	}
	return ""
}

// calleeDiagram draws fn's callee tree: interface calls go to the method,
// and from it, dotted, to its implementations.
func calleeDiagram(p *printer, fn *graph.Function, nodes []*query.Node) *diagram {
	b := newDiagramBuilder(p)
	var walk func(src int, nodes []*query.Node)
	walk = func(src int, nodes []*query.Node) {
		for _, n := range nodes {
			e := dedge{from: src, label: b.label(n.Site)}
			switch n.Kind {
			case query.Interface:
				e.to = b.fn(n.Target, ifaceNode)
			case query.Implementation:
				e.to, e.dotted = b.fn(n.Target, funcNode), true
			case query.Dynamic:
				e.to = b.dynamic(src, n.Call)
			default:
				e.to = b.fn(n.Target, funcNode)
			}
			b.edge(e)
			walk(e.to, n.Children)
		}
	}
	walk(b.fn(fn.ID, funcNode), nodes)
	return b.diagram
}

// callerDiagram draws fn's caller tree, edges pointing at the callee: an
// interface call goes through the method it calls.
func callerDiagram(p *printer, fn *graph.Function, nodes []*query.Node) *diagram {
	b := newDiagramBuilder(p)
	var walk func(dst int, nodes []*query.Node)
	walk = func(dst int, nodes []*query.Node) {
		for _, n := range nodes {
			from := b.fn(n.From, funcNode)
			if n.Kind == query.Interface {
				m := b.fn(n.Call.Callee, ifaceNode)
				b.edge(dedge{from: from, to: m})
				b.edge(dedge{from: m, to: dst, dotted: true})
			} else {
				b.edge(dedge{from: from, to: dst, label: b.label(n.Site)})
			}
			walk(from, n.Children)
		}
	}
	walk(b.fn(fn.ID, funcNode), nodes)
	return b.diagram
}

// write prints d as format, mermaid or dot; name is the DOT graph's.
func (d *diagram) write(w, errw io.Writer, format, name string) error {
	var b strings.Builder
	if format == "dot" {
		d.dot(&b, name)
	} else {
		d.mermaid(&b)
		if len(d.nodes) > mermaidNodes {
			fmt.Fprintf(errw, "%d nodes: Mermaid renders badly past about %d; try a smaller --depth, or --direct\n", len(d.nodes), mermaidNodes)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func (d *diagram) mermaid(b *strings.Builder) {
	b.WriteString("flowchart LR\n")
	var outside []string
	for i, n := range d.nodes {
		open, close := `["`, `"]`
		switch n.kind {
		case ifaceNode:
			open, close = `{{"`, `"}}`
		case dynamicNode:
			open, close = `(["`, `"])`
		}
		fmt.Fprintf(b, "  n%d%s%s%s\n", i, open, mermaidText(n.label), close)
		if n.outside {
			outside = append(outside, fmt.Sprintf("n%d", i))
		}
	}
	for _, e := range d.edges {
		switch {
		case e.dotted:
			fmt.Fprintf(b, "  n%d -.-> n%d\n", e.from, e.to)
		case e.label != "":
			fmt.Fprintf(b, "  n%d -- \"%s\" --> n%d\n", e.from, mermaidText(e.label), e.to)
		default:
			fmt.Fprintf(b, "  n%d --> n%d\n", e.from, e.to)
		}
	}
	b.WriteString("  classDef root stroke-width:3px\n")
	if len(outside) > 0 {
		b.WriteString("  classDef outside stroke-dasharray:4 3\n")
	}
	b.WriteString("  class n0 root\n")
	if len(outside) > 0 {
		fmt.Fprintf(b, "  class %s outside\n", strings.Join(outside, ","))
	}
}

func (d *diagram) dot(b *strings.Builder, name string) {
	fmt.Fprintf(b, "digraph %s {\n  rankdir=LR;\n  node [shape=box];\n", name)
	for i, n := range d.nodes {
		attrs := []string{"label=" + dotText(n.label)}
		if i == 0 {
			attrs = append(attrs, "penwidth=2")
		}
		switch n.kind {
		case ifaceNode:
			attrs = append(attrs, "shape=hexagon")
		case dynamicNode:
			attrs = append(attrs, "shape=ellipse")
		}
		if n.outside {
			attrs = append(attrs, "style=dashed")
		}
		fmt.Fprintf(b, "  %s [%s];\n", dotText(n.id), strings.Join(attrs, ", "))
	}
	for _, e := range d.edges {
		var attrs []string
		if e.dotted {
			attrs = append(attrs, "style=dotted")
		}
		if e.label != "" {
			attrs = append(attrs, "label="+dotText(e.label))
		}
		fmt.Fprintf(b, "  %s -> %s", dotText(d.nodes[e.from].id), dotText(d.nodes[e.to].id))
		if len(attrs) > 0 {
			fmt.Fprintf(b, " [%s]", strings.Join(attrs, ", "))
		}
		b.WriteString(";\n")
	}
	b.WriteString("}\n")
}

// mermaidText escapes a label for inside Mermaid's quotes, where # starts
// an entity.
var mermaidText = strings.NewReplacer("#", "#35;", `"`, "#quot;", "<", "#lt;", ">", "#gt;", "&", "#amp;").Replace

// dotText is s as a quoted DOT ID.
func dotText(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
