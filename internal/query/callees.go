// callees.go: what a function reaches, and trees of it (docs/output.md,
// Trees).

package query

import (
	"cmp"
	"slices"

	"github.com/samippp/grepo/internal/graph"
)

// Callees returns every site in fn that reaches a function, by position:
// its calls (static, through an interface, through a local, one site per
// candidate; or dynamic, when nothing resolves it), and the functions it
// passes or stores as values, which may be called later.
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
	// Stable: a call stays before the values passed to it.
	slices.SortStableFunc(sites, func(a, b Site) int { return comparePos(a.Pos, b.Pos) })
	return sites
}

// Implementations returns the methods a call to interface method m can run,
// one site per method, by position: for each type implementing m's
// interface, its method of that name, or the one promoted to it. A method
// outside the repo (promoted from an embedded *os.File) is located at the
// first type it's promoted to.
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

// Node is a site in a tree, with the sites under it.
type Node struct {
	Site
	Children  []*Node
	Seen      bool // its function was expanded earlier in the tree: children left out
	Recursive bool // its function is on the path above it: children left out
}

// CalleeTree is fn's callees and theirs, depth levels down (0: all the way).
// An interface call's children are the methods it can run (Implementations),
// and functions outside the repo are leaves. With direct, only static calls.
func (ix *Index) CalleeTree(fn *graph.Function, depth int, direct bool) []*Node {
	callees := func(id string) []Site {
		f := ix.funcs[id]
		if f == nil || f.Pos == "" { // outside the repo: its calls aren't in the graph
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

// tree builds Nodes depth-first. A site's function (its Target) is expanded
// once: later sites reaching it are Seen, and one reaching a function on the
// path above it is Recursive.
type tree struct {
	depth    int
	children func(Site) []Site
	path     map[string]bool // functions being expanded, from the root down
	expanded map[string]bool
}

func (t *tree) nodes(sites []Site, level int) []*Node {
	var nodes []*Node
	for _, s := range sites {
		n := &Node{Site: s}
		nodes = append(nodes, n)
		switch key := s.Target; {
		case key == "":
		case t.path[key]: // on the path, so being expanded: it has children
			n.Recursive = true
		case t.expanded[key]: // marked even at the depth limit: its children are above
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
