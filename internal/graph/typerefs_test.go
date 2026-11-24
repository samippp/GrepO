package graph

import (
	"fmt"
	"strings"
	"testing"
)

// Type refs, checked exactly for store/typerefs.go.
func TestCallGraphTypeRefs(t *testing.T) {
	g := buildFixtureCallGraph(t)

	ref := storePkg + ".Ref"
	refs := storePkg + ".Refs"
	alias := storePkg + ".RefAlias"
	ordered := storePkg + ".Ordered"
	in := storePkg + ".refsIn"
	r := func(referrer, typ, line string) TypeRef {
		return TypeRef{Referrer: referrer, Type: typ, Pos: "store/typerefs.go:" + line}
	}

	want := []TypeRef{
		// Package-level declarations, by the package: fields (embedded and
		// outside ones too), an underlying type, an alias, a constraint, an
		// interface method's signature, a var's literal.
		r(storePkg, ref, "12:8"),
		r(storePkg, modelPkg+".User", "13:2"),
		r(storePkg, "net/http.Request", "14:8"),
		r(storePkg, refs, "15:7"),
		r(storePkg, ref, "18:13"),
		r(storePkg, ref, "20:17"),
		r(storePkg, ordered, "24:15"), // T is a type param: not a ref
		r(storePkg, ref, "27:9"),
		r(storePkg, alias, "27:14"),
		r(storePkg, ref, "30:18"),
		// 32: a receiver isn't a ref.

		// A signature (any is built in), then a body: a var's type, a
		// literal, a conversion through an alias, new and make, an assertion,
		// type switch cases, a generic instance, a method expression, a
		// local type and a var of it.
		r(in, "net/http.Handler", "34:22"),
		r(in, ref, "34:37"),
		r(in, ref, "35:8"),
		r(in, ref, "36:9"),
		r(in, alias, "37:6"),
		r(in, ref, "38:13"),
		r(in, ref, "38:26"),
		r(in, ref, "39:12"),
		r(in, ref, "41:8"),
		r(in, refs, "41:13"),
		r(in, storePkg+".Sorted[T]", "43:6"),
		r(in, ref, "44:8"),
		r(in, ref, "45:23"),
		r(in, in+".local#1", "46:8"),
		// A closure's signature is the closure's.
		r(in+"$1", ref, "47:13"),
		r(in+"$1", modelPkg+".User", "47:18"),

		// A generic function's constraint.
		r(storePkg+".smallest", ordered, "51:17"),
	}

	var got []TypeRef
	for _, x := range g.TypeRefs {
		if strings.HasPrefix(x.Pos, "store/typerefs.go:") {
			got = append(got, x)
		}
	}
	checkExact(t, "type ref", got, want, formatTypeRef)
}

// The types other fixture files name are refs too, outside ones included,
// and none is a built-in type or a type param.
func TestCallGraphTypeRefsElsewhere(t *testing.T) {
	g := buildFixtureCallGraph(t)
	seen := map[string]bool{}
	for _, x := range g.TypeRefs {
		seen[formatTypeRef(x)] = true
		if !strings.Contains(x.Type, ".") {
			t.Errorf("type ref to %s at %s: built in or a type param", x.Type, x.Pos)
		}
	}
	for _, want := range []TypeRef{
		{Referrer: storePkg + ".lock", Type: tRLocked, Pos: "store/reads.go:114:14"},
		{Referrer: storePkg, Type: "sync.Mutex", Pos: "store/reads.go:39:21"},
		{Referrer: storePkg, Type: "time.Duration", Pos: "store/consts.go:16:13"}, // time is a dependency only through it
	} {
		if !seen[formatTypeRef(want)] {
			t.Errorf("missing type ref %s", formatTypeRef(want))
		}
	}
}

func formatTypeRef(x TypeRef) string {
	return fmt.Sprintf("%s → %s at %s", x.Referrer, x.Type, x.Pos)
}
