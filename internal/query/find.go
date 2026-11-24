package query

import (
	"cmp"
	"slices"
	"strings"

	"github.com/samippp/grepo/internal/graph"
)

// DeclKind is what a declaration find lists is.
type DeclKind string

const (
	FuncDecl      DeclKind = "func"
	MethodDecl    DeclKind = "method"
	StructDecl    DeclKind = "struct"
	InterfaceDecl DeclKind = "interface"
	AliasDecl     DeclKind = "alias"
	TypeDecl      DeclKind = "type" // any other named type
	FieldDecl     DeclKind = "field"
	VarDecl       DeclKind = "var"
	ConstDecl     DeclKind = "const"
)

// DeclKinds is every DeclKind.
var DeclKinds = []DeclKind{FuncDecl, MethodDecl, StructDecl, InterfaceDecl, AliasDecl, TypeDecl, FieldDecl, VarDecl, ConstDecl}

var typeDecls = map[graph.TypeKind]DeclKind{
	graph.StructType: StructDecl, graph.InterfaceType: InterfaceDecl, graph.AliasType: AliasDecl, graph.OtherType: TypeDecl,
}

// Match is a declaration find lists, with how much it's used.
type Match struct {
	Kind DeclKind
	ID   string
	Pos  string
	Uses int // what matches rank by: Callers, Writes+Reads, or Refs

	Callers int // func, method: Callers' sites, but those through an interface
	Via     int // the sites through an interface

	Writes int // field, var, const: assign, update, content, construct
	Reads  int // read, escape

	Refs            int // types: TypeRefs
	Methods         int // methods declared with it as Owner
	Implementations int // interface: repo types implementing it
}

// Find returns the repo's declarations of the kinds given (nil: all) that
// word matches, most used first, ties by position. A word without a dot
// matches a declaration's own name; one with a dot, its short name with
// ( ) * and type args removed. Both are case-insensitive substrings.
// Closures and local types aren't listed.
func (ix *Index) Find(word string, kinds []DeclKind) []Match {
	qualified := strings.Contains(word, ".")
	if qualified {
		word = CleanName(word)
	}
	word = strings.ToLower(word)
	want := func(k DeclKind, name, id string) bool {
		if kinds != nil && !slices.Contains(kinds, k) {
			return false
		}
		if qualified {
			name = CleanName(ix.Short(id))
		}
		return strings.Contains(strings.ToLower(name), word)
	}

	var out []Match
	for _, fs := range ix.byName {
		for _, f := range fs {
			k := FuncDecl
			if f.Owner != "" {
				k = MethodDecl
			}
			if !want(k, f.Name, f.ID) {
				continue
			}
			m := Match{Kind: k, ID: f.ID, Pos: f.Pos}
			for _, s := range ix.Callers(f) {
				if s.Kind == Interface {
					m.Via++
				} else {
					m.Callers++
				}
			}
			m.Uses = m.Callers
			out = append(out, m)
		}
	}
	for _, t := range ix.types {
		if strings.Contains(t.ID, "#") { // a local type
			continue
		}
		if k := typeDecls[t.Kind]; want(k, t.Name, t.ID) {
			m := Match{Kind: k, ID: t.ID, Pos: t.Pos, Refs: ix.refs[t.ID], Methods: ix.methodCount[t.ID]}
			if k == InterfaceDecl {
				m.Implementations = len(ix.ifaces[t.ID])
			}
			m.Uses = m.Refs
			out = append(out, m)
		}
		for _, fd := range t.Fields {
			if want(FieldDecl, fd.Name, fd.ID) {
				out = append(out, ix.valueMatch(FieldDecl, fd.ID, fd.Pos))
			}
		}
	}
	for _, v := range ix.vars {
		k := VarDecl
		if v.Const {
			k = ConstDecl
		}
		if want(k, v.Name, v.ID) {
			out = append(out, ix.valueMatch(k, v.ID, v.Pos))
		}
	}
	slices.SortFunc(out, func(a, b Match) int {
		return cmp.Or(cmp.Compare(b.Uses, a.Uses), comparePos(a.Pos, b.Pos), cmp.Compare(a.ID, b.ID))
	})
	return out
}

func (ix *Index) valueMatch(k DeclKind, id, pos string) Match {
	m := Match{Kind: k, ID: id, Pos: pos, Writes: ix.writes[id], Reads: ix.reads[id]}
	m.Uses = m.Writes + m.Reads
	return m
}

// CleanName is a name with ( ) * and type args removed: (*store.List[T]).Push → store.List.Push.
func CleanName(s string) string {
	return stripTypeArgs(strings.NewReplacer("(", "", ")", "", "*", "").Replace(s))
}
