package graph

import (
	"strings"
	"testing"
)

// Reads and escapes, checked exactly for store/reads.go only: every other
// fixture file has reads too, and they aren't listed.
func TestCallGraphReads(t *testing.T) {
	g := buildFixtureCallGraph(t)

	r := func(accessor, target, line string) Access {
		return Access{Accessor: accessor, Target: target, Pos: rat(line), Kind: ReadAccess}
	}
	e := func(accessor, target, line string) Access {
		return Access{Accessor: accessor, Target: target, Pos: rat(line), Kind: EscapeAccess}
	}
	idWritesR := "(*" + tRHolder + ").writes"
	idAddrs := "(*" + tRHolder + ").addresses"
	idOutsides := "(*" + tRHolder + ").outsides"
	maxUsers := storePkg + ".maxUsers"

	want := []Access{
		// Package-level: the package reads the const, in a type, a var and a const.
		r(storePkg, maxUsers, "44:13"),
		r(storePkg, maxUsers, "46:13"),
		r(storePkg, maxUsers, "48:17"),
		// p.X++ is an update only; p.X in Value is a read.
		r("("+tRPart+").Value", tRPart+".X", "15:36"),

		// values: plain uses, range, index, every hop, embedded hops, builtins.
		r(idValues, tRHolder+".N", "63:7"),
		r(idValues, tRHolder+".N", "64:6"),
		r(idValues, tRHolder+".N", "65:5"),
		r(idValues, tRHolder+".Items", "67:12"),
		r(idValues, tRHolder+".Items", "69:7"),
		r(idValues, tRHolder+".Key", "69:15"),
		r(idValues, tRPart+".X", "70:7"),
		r(idValues, tRHolder+".Val", "70:7"),
		r(idValues, tRPart+".X", "71:7"),
		r(idValues, tRHolder+".Ptr", "71:7"), // pointer hops are reads too
		r(idValues, tRBase+".ID", "72:7"),
		r(idValues, tRHolder+".Base", "72:7"), // r.ID is r.Base.ID
		r(idValues, tRHolder+".Items", "73:11"),
		r(idValues, tRHolder+".Items", "74:11"), // copy's source
		r(idValues, tRHolder+".N", "75:6"),
		r(idValues, tRHolder+".N", "75:11"),         // the same read twice on a line: two sites
		r(idValues+"$1", tRHolder+".Step", "76:16"), // in a closure: belongs to it

		// writes: no read where the path gets a write edge.
		// 81, 82: nothing. 83: the write stops at Sub.Ptr, so r.Sub is read.
		r(idWritesR, tRHolder+".Sub", "83:2"),
		r(idWritesR, tRHolder+".Key", "84:10"), // the index
		// 85: update only. 86: the matching r.N is suppressed, r.Step isn't.
		r(idWritesR, tRHolder+".Step", "86:14"),
		// 87: update only; append is a builtin, so no escape either.

		// addresses: & is an escape, and so is every struct-value hop up to the
		// first pointer (no reads on them); the pointer and hops before it are read.
		e(idAddrs, tRHolder+".N", "91:8"),
		e(idAddrs, tRPart+".X", "92:8"),
		e(idAddrs, tRHolder+".Val", "92:8"), // Val's memory holds X
		e(idAddrs, tRPart+".X", "93:8"),
		r(idAddrs, tRHolder+".Ptr", "93:8"), // the pointer is loaded
		e(idAddrs, storePkg+".shared", "95:7"),

		// calls: a read unless Go takes the field's address for a pointer method.
		r(idCalls, tRHolder+".Ptr", "99:2"),
		e(idCalls, tRHolder+".Val", "100:2"),
		r(idCalls, tRHolder+".Val", "101:2"),
		r(idCalls, tRHolder+".H", "102:2"),
		r(idCalls, tRHolder+".OnErr", "103:2"),
		e(idCalls, tRHolder+".mu", "104:2"),
		// 105: r is a param, nothing.
		e(idCalls, storePkg+".shared", "106:2"),
		r(idCalls, storePkg+".OnReady", "107:2"),
		e(idCalls, tRHolder+".Val", "108:7"), // method value, address case
		r(idCalls, tRHolder+".Ptr", "109:7"),
		e(storePkg+".lock", tRLocked+".Mutex", "115:2"), // promoted through the embedded Mutex

		// outsides: repo consts are read; outside consts get nothing; outside vars and fields are kept.
		r(idOutsides, maxUsers, "119:7"),
		// 120: http.StatusOK, nothing.
		r(idOutsides, "os.Args", "121:11"),
		r(idOutsides, "net/http.Request.Method", "122:11"),
		r(idOutsides, "net/url.URL.Path", "122:24"),
		r(idOutsides, "net/http.Request.URL", "122:24"),
		// 123-124: a local const, nothing.

		// hands: a slice or map field passed, stored, sent or returned is read and escapes.
		r(idHands, tRHolder+".Items", "128:10"),
		e(idHands, tRHolder+".Items", "128:10"),
		r(idHands, tRHolder+".Users", "129:7"),
		e(idHands, tRHolder+".Users", "129:7"),
		r(idHands, tRHolder+".Items", "130:16"),
		e(idHands, tRHolder+".Items", "130:16"),
		r(idHands, tRHolder+".Items", "131:20"), // into a literal
		e(idHands, tRHolder+".Items", "131:20"),
		r(idHands, tRHolder+".Items", "132:8"),
		e(idHands, tRHolder+".Items", "132:8"),
		r(idHands, tRHolder+".Ptr", "133:7"),    // pointers don't escape
		r(idHands, tRHolder+".Items", "134:10"), // r.Items[1:] isn't the field itself
		r(idHands, tRHolder+".Users", "136:9"),
		e(idHands, tRHolder+".Users", "136:9"),

		// The generic field, like writes.
		r(storePkg+".bagLen", tRBag+".items", "139:43"),

		// nested: escapes reach every enclosing struct value, like writes' content.
		e(idNested, tRPart+".X", "148:7"),
		e(idNested, tRHolder+".Val", "148:7"),
		e(idNested, tROuter+".In", "148:7"),
		e(idNested, tRHolder+".Val", "149:2"), // pointer method on a nested value
		e(idNested, tROuter+".In", "149:2"),
		e(idNested, tRLocked+".Mutex", "150:2"), // promoted, on a value field
		e(idNested, tROuter+".L", "150:2"),
		e(idNested, tRBase+".ID", "151:7"), // embedded hops too
		e(idNested, tRHolder+".Base", "151:7"),
		e(idNested, tROuter+".In", "151:7"),
		e(idNested, tRPart+".X", "152:7"),
		e(idNested, tRHolder+".Val", "152:7"),
		e(idNested, storePkg+".shared", "152:7"), // the package var holding it
		e(idNested, tRPart+".X", "153:7"),
		r(idNested, tRHolder+".Ptr", "153:7"), // stops at the pointer: it and the hops before it are read
		r(idNested, tROuter+".In", "153:7"),

		// Consts in types: the param and result types, a local var's type, a
		// closure's signature, a literal's type; a package var's type, by the package.
		r(storePkg+".sized", maxUsers, "157:15"),
		r(storePkg+".sized", maxUsers, "157:31"),
		r(storePkg+".sized", maxUsers, "158:9"),
		r(storePkg+".sized$1", maxUsers, "159:14"), // the closure's own signature
		r(storePkg+".sized", maxUsers, "161:10"),
		r(storePkg, maxUsers, "164:12"),

		// A repo const from another package (store/consts.go): read too.
		{Accessor: storePkg + ".tagLimit", Target: modelPkg + ".MaxTag", Pos: "store/consts.go:10:30", Kind: ReadAccess},
		// In a generic type's constraint, by the package (#69).
		{Accessor: storePkg, Target: modelPkg + ".MaxTag", Pos: "store/consts.go:13:18", Kind: ReadAccess},
	}

	var got []Access
	for _, a := range g.Accesses {
		if (a.Kind == ReadAccess || a.Kind == EscapeAccess) && (strings.HasPrefix(a.Pos, "store/reads.go:") || strings.HasPrefix(a.Pos, "store/consts.go:")) {
			got = append(got, a)
		}
	}
	checkExact(t, "access", got, want, formatAccess)
}
