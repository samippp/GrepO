package query

import (
	"cmp"
	"slices"
	"strings"

	"github.com/samippp/grepo/internal/graph"
)

// Site kinds for state: how a site accesses a field or var.
const (
	Write  Kind = "write"  // assign, update, content or construct
	Escape Kind = "escape" // hands out access, so a change can happen elsewhere
	Read   Kind = "read"
)

// Target is a repo type, field or package var or const that state asks about.
type Target struct {
	Kind DeclKind
	ID   string
	Pos  string

	Type  *graph.Type     // a type; a field's owner
	Field *graph.Field    // a field
	Var   *graph.Variable // a var or const
}

func (t Target) pkg() string {
	if t.Var != nil {
		return t.Var.Package
	}
	return t.Type.Package
}

type fieldRef struct {
	owner *graph.Type
	field *graph.Field
}

func typeTarget(t *graph.Type) Target {
	return Target{Kind: typeDecls[t.Kind], ID: t.ID, Pos: t.Pos, Type: t}
}

func fieldTarget(r fieldRef) Target {
	return Target{Kind: FieldDecl, ID: r.field.ID, Pos: r.field.Pos, Type: r.owner, Field: r.field}
}

func varTarget(v *graph.Variable) Target {
	k := VarDecl
	if v.Const {
		k = ConstDecl
	}
	return Target{Kind: k, ID: v.ID, Pos: v.Pos, Var: v}
}

// Target returns the repo type, field or var name means: an ID, or a name
// qualified as much as needed (Store, Store.users, store.Store.users, a path
// suffix), type args ignored, as Func takes them. An alias of a repo struct
// is that struct. Of several matches, the only one in package prefer wins.
// Local types and their fields match by ID only.
func (ix *Index) Target(name, prefer string) (Target, error) {
	t, err := ix.target(name, prefer)
	if err == nil && t.Kind == AliasDecl {
		if to := ix.types[t.Type.Target]; to != nil && to.Kind == graph.StructType {
			return typeTarget(to), nil
		}
	}
	return t, err
}

func (ix *Index) target(name, prefer string) (Target, error) {
	if t := ix.types[name]; t != nil {
		return typeTarget(t), nil
	}
	if r, ok := ix.fields[name]; ok {
		return fieldTarget(r), nil
	}
	if v := ix.varByID[name]; v != nil {
		return varTarget(v), nil
	}
	clean := CleanName(name)
	qual, last := "", clean
	if i := strings.LastIndex(clean, "."); i >= 0 {
		qual, last = clean[:i], clean[i+1:]
	}

	var matches []Target
	for _, t := range ix.typesByName[last] {
		if ix.qualifies("", t.Package, qual) {
			matches = append(matches, typeTarget(t))
		}
	}
	for _, r := range ix.fieldsByName[last] {
		if ix.qualifies(r.owner.ID, r.owner.Package, qual) {
			matches = append(matches, fieldTarget(r))
		}
	}
	for _, v := range ix.varsByName[last] {
		if ix.qualifies("", v.Package, qual) {
			matches = append(matches, varTarget(v))
		}
	}
	switch len(matches) {
	case 0:
		return Target{}, &NotFoundError{Name: name, Module: ix.module, State: true}
	case 1:
		return matches[0], nil
	}
	if t, ok := only(matches, func(t Target) bool { return t.pkg() == prefer }); ok {
		return t, nil
	}
	slices.SortFunc(matches, func(a, b Target) int { return cmp.Compare(a.ID, b.ID) })
	return Target{}, &AmbiguousError{Name: name, Targets: matches}
}

// Role is what a field's writes say it is, when they settle it.
type Role string

const (
	NoWrites          Role = "no writes found"
	SetAtConstruction Role = "set at construction" // construct writes only
	Counter           Role = "counter"             // a number, update writes only
)

// FieldState is a field's writes by kind, and its escapes.
type FieldState struct {
	Field   *graph.Field
	Writes  map[graph.AccessKind]int // assign, update, content, construct; no zeros
	Escapes int
	Role    Role // "" when the writes don't settle one
}

// Fields returns t's fields, in declaration order, with their writes.
func (ix *Index) Fields(t *graph.Type) []FieldState {
	out := make([]FieldState, len(t.Fields))
	byID := map[string]*FieldState{}
	for i := range t.Fields {
		out[i] = FieldState{Field: &t.Fields[i], Writes: map[graph.AccessKind]int{}}
		byID[t.Fields[i].ID] = &out[i]
	}
	for _, a := range ix.accesses {
		f := byID[a.Target]
		switch {
		case f == nil, a.Kind == graph.ReadAccess:
		case a.Kind == graph.EscapeAccess:
			f.Escapes++
		default:
			f.Writes[a.Kind]++
		}
	}
	for i := range out {
		out[i].Role = role(out[i].Writes, out[i].Field.Type)
	}
	return out
}

func role(writes map[graph.AccessKind]int, typ string) Role {
	switch n := len(writes); {
	case n == 0:
		return NoWrites
	case n > 1:
		return ""
	case writes[graph.ConstructAccess] > 0:
		return SetAtConstruction
	case writes[graph.UpdateAccess] > 0 && numeric[typ]:
		return Counter
	}
	return ""
}

var numeric = map[string]bool{
	"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true, "uintptr": true,
	"float32": true, "float64": true,
}

// Accesses returns the sites that write the field or var id or hand out
// access to it, and with reads, those that read it; by position.
func (ix *Index) Accesses(id string, reads bool) []Site {
	var sites []Site
	for i := range ix.accesses {
		a := &ix.accesses[i]
		if a.Target != id {
			continue
		}
		k := Write
		switch a.Kind {
		case graph.EscapeAccess:
			k = Escape
		case graph.ReadAccess:
			if !reads {
				continue
			}
			k = Read
		}
		sites = append(sites, Site{Kind: k, Pos: a.Pos, From: a.Accessor, Access: a})
	}
	slices.SortStableFunc(sites, func(a, b Site) int {
		return cmp.Or(comparePos(a.Pos, b.Pos), cmp.Compare(a.From, b.From))
	})
	return sites
}
