// callees.go: what a function reaches, and trees of it.

package query

import (
	"cmp"
	"slices"

	"github.com/samippp/grepo/internal/graph"
)

// Callees returns every site in fn that reaches a function, by position:
// calls (a site per local's candidate) and functions passed or stored.
func (ix *Index) Callees(fn *graph.Function) []Site {
	var sites []Site
	for _, c := range ix.byCaller[fn.ID] {
		at := Site{Pos: c.Pos, From: fn.ID, Call: c}
		switch {
		case c.CallKind == graph.InterfaceCall:
			at.Kind, at.Target = Interface, c.Callee
			if m := ix.funcs[c.Callee]; m != nil {
				at.Via = m.Owner
			}
			sites = append(sites, at)
		case c.CallKind == graph.DynamicCall && len(c.Funcs) > 0:
			for _, f := range c.Funcs {
				at.Kind, at.Target = Local, f
				sites = append(sites, at)
			}
		case c.CallKind == graph.DynamicCall:
			at.Kind = Dynamic
			sites = append(sites, at)
		default:
			at.Kind, at.Target = Direct, c.Callee
			sites = append(sites, at)
		}
		for _, a := range c.Args {
			if a.Func != "" {
				sites = append(sites, Site{Kind: Value, Pos: c.Pos, From: fn.ID, Via: c.Callee, Call: c, Target: a.Func})
			}
		}
	}
	for _, a := range ix.stores[fn.ID] {
		sites = append(sites, Site{Kind: Value, Pos: a.Pos, From: fn.ID, Via: a.Target, Access: a, Target: a.Func})
	}
	// Stable: a call stays before its values.
	slices.SortStableFunc(sites, func(a, b Site) int { return comparePos(a.Pos, b.Pos) })
	return sites
}

// Implementations returns the methods a call to interface method m can run,
// by position. One promoted from outside the repo is at its type.
func (ix *Index) Implementations(m string) []Site {
	im := ix.funcs[m]
	if im == nil || im.Owner == "" {
		return nil
	}
	var sites []Site
	seen := map[string]bool{}
	for _, impl := range ix.ifaces[im.Owner] {
		id := ix.methods[method{impl.Type, im.Name}]
		for _, p := range impl.Promoted {
			if f := ix.funcs[p]; f != nil && f.Name == im.Name {
				id = p
			}
		}
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		pos := ""
		if f := ix.funcs[id]; f != nil {
			pos = f.Pos
		}
		if t := ix.types[impl.Type]; pos == "" && t != nil {
			pos = t.Pos
		}
		sites = append(sites, Site{Kind: Implementation, Pos: pos, From: m, Via: im.Owner, Target: id})
	}
	slices.SortFunc(sites, func(a, b Site) int { return cmp.Or(comparePos(a.Pos, b.Pos), cmp.Compare(a.Target, b.Target)) })
	return sites
}

// Node is a site in a tree.
type Node struct {
	Site
	Children  []*Node
	Seen      bool // expanded earlier: no children
	Recursive bool // on the path above: no children
}

// CalleeTree is fn's callees, depth levels down (0: all). An interface
// call's children are its Implementations; direct keeps static calls.
func (ix *Index) CalleeTree(fn *graph.Function, depth int, direct bool) []*Node {
	callees := func(id string) []Site {
		f := ix.funcs[id]
		if f == nil || f.Pos == "" { // outside the repo
			return nil
		}
		sites := ix.Callees(f)
		if direct {
			sites = slices.DeleteFunc(sites, func(s Site) bool { return s.Kind != Direct })
		}
		return sites
	}
	t := &tree{
		depth:    depth,
		key:      func(s Site) string { return s.Target },
		path:     map[string]bool{fn.ID: true},
		expanded: map[string]bool{},
		children: func(s Site) []Site {
			if s.Kind == Interface {
				return ix.Implementations(s.Target)
			}
			if s.Target == "" {
				return nil
			}
			return callees(s.Target)
		},
	}
	return t.nodes(callees(fn.ID), 1)
}

// CallerTree is the sites that can reach fn, depth levels down (0: all): a
// site's children are its From's callers. direct keeps direct calls.
func (ix *Index) CallerTree(fn *graph.Function, depth int, direct bool) []*Node {
	callers := func(id string) []Site {
		f := ix.funcs[id]
		if f == nil { // package-level code
			return nil
		}
		sites := ix.Callers(f)
		if direct {
			sites = slices.DeleteFunc(sites, func(s Site) bool { return s.Kind != Direct })
		}
		return sites
	}
	t := &tree{
		depth:    depth,
		key:      func(s Site) string { return s.From },
		children: func(s Site) []Site { return callers(s.From) },
		path:     map[string]bool{fn.ID: true},
		expanded: map[string]bool{},
	}
	return t.nodes(callers(fn.ID), 1)
}

// tree builds Nodes depth-first, expanding each site's key once.
type tree struct {
	depth    int
	key      func(Site) string // the function a site's children belong to
	children func(Site) []Site
	path     map[string]bool // being expanded
	expanded map[string]bool
}

func (t *tree) nodes(sites []Site, level int) []*Node {
	var nodes []*Node
	for _, s := range sites {
		n := &Node{Site: s}
		nodes = append(nodes, n)
		switch key := t.key(s); {
		case key == "":
		case t.path[key]:
			n.Recursive = true
		case t.expanded[key]: // even at the depth limit
			n.Seen = true
		case t.depth > 0 && level >= t.depth:
		default:
			if kids := t.children(s); len(kids) > 0 {
				t.expanded[key], t.path[key] = true, true
				n.Children = t.nodes(kids, level+1)
				delete(t.path, key)
			}
		}
	}
	return nodes
}
