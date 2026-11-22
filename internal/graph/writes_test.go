package graph

import "testing"

func TestCallGraphAccesses(t *testing.T) {
	g := buildFixtureCallGraph(t)

	idWrites := "(*" + tWBox + ").Writes"
	w := func(accessor, target, pos string, kind AccessKind) Access {
		return Access{Accessor: accessor, Target: target, Pos: pos, Kind: kind}
	}
	wf := func(accessor, target, pos string, kind AccessKind, fn string) Access {
		a := w(accessor, target, pos, kind)
		a.Func = fn
		return a
	}
	idWritesR := "(*" + tRHolder + ").writes"

	want := []Access{
		// Existing fixture code. Literals construct their fields; package vars with a value
		// are constructed by the package. Timeout has no value, so no edge.
		w(idNew, tStore+".users", "store/store.go:21:35", ConstructAccess),
		w("("+tCache+").Get", tUser+".ID", "store/cache.go:9:59", ConstructAccess),
		w(storePkg, storePkg+".defaultStore", "store/vars.go:4:5", ConstructAccess),
		w(storePkg, storePkg+".a", "store/vars.go:8:5", ConstructAccess),
		w(storePkg, storePkg+".b", "store/vars.go:8:8", ConstructAccess),
		w(storePkg, storePkg+".first", "store/vars.go:11:5", ConstructAccess),
		w(storePkg, storePkg+".err", "store/vars.go:11:12", ConstructAccess),
		// l.items = append(l.items, v) through List[T]: the generic field, and an update.
		w(idPush, tList+".items", "main.go:56:31", UpdateAccess),
		// A local type's field.
		w(mainPkg+".wrap", tBox+".v", "main.go:115:10", ConstructAccess),

		// var defaultBox = Box{Name: "d"}: the var and the literal's field.
		w(storePkg, storePkg+".defaultBox", wat("28:5"), ConstructAccess),
		w(storePkg, tWBox+".Name", wat("28:22"), ConstructAccess),

		w(idWrites, tWBox+".N", wat("31:2"), AssignAccess),
		w(idWrites, tWBox+".N", wat("32:2"), UpdateAccess), // ++
		w(idWrites, tWBox+".N", wat("33:2"), UpdateAccess), // +=
		w(idWrites, tWBox+".N", wat("34:2"), UpdateAccess), // b.N = b.N + 1: same expression on the right
		w(idWrites, tWBox+".N", wat("35:2"), AssignAccess), // b.N = o.N + 1: same field ID, different object
		w(idWrites, tWBox+".Items", wat("36:2"), UpdateAccess),
		// b.Users[1] = model.User{}: an empty literal constructs nothing.
		w(idWrites, tWBox+".Users", wat("37:2"), ContentAccess),
		w(idWrites, tWBox+".Users", wat("38:2"), ContentAccess),
		w(idWrites, tUser+".Name", wat("38:26"), ConstructAccess),
		w(idWrites, tWBox+".Items", wat("39:2"), ContentAccess), // b.Items[0]++: the index decides
		w(idWrites, tWBox+".Users", wat("40:2"), ContentAccess), // delete
		w(idWrites, tWBox+".Items", wat("41:2"), ContentAccess), // clear
		w(idWrites, tWBox+".Buf", wat("42:2"), ContentAccess),   // copy(b.Buf[:], ...): a slice of the field
		w(idWrites, tWBox+".Buf", wat("43:2"), ContentAccess),
		// b.Inner.X = 1: Inner is a struct value, so Box's memory changes too.
		w(idWrites, tWInner+".X", wat("44:2"), AssignAccess),
		w(idWrites, tWBox+".Inner", wat("44:2"), ContentAccess),
		// b.Ptr.X = 1: Ptr still holds the same address, but what it points to changed,
		// as with a slice or map. The walk stops there.
		w(idWrites, tWInner+".X", wat("45:2"), AssignAccess),
		w(idWrites, tWBox+".Ptr", wat("45:2"), ContentAccess),
		w(idWrites, tWBox+".Ptr", wat("46:2"), ContentAccess), // *b.Ptr = Inner{}
		// b.Tags[0].Name = "t": Name is promoted from the embedded User value; the index
		// into Tags writes its contents and stops the path.
		w(idWrites, tUser+".Name", wat("47:2"), AssignAccess),
		w(idWrites, tTagged+".User", wat("47:2"), ContentAccess),
		w(idWrites, tWBox+".Tags", wat("47:2"), ContentAccess),
		w(idWrites, storePkg+".Counter", wat("48:2"), AssignAccess),
		// b.N, Counter = 4, 5: one edge per left-hand item.
		w(idWrites, tWBox+".N", wat("49:2"), AssignAccess),
		w(idWrites, storePkg+".Counter", wat("49:7"), AssignAccess),

		// Field writes count whatever the root; writes to locals themselves don't.
		// Nothing for local := Box{} (empty), p := &local, or *p = Box{}.
		w(storePkg+".locals", tWBox+".N", wat("54:2"), AssignAccess),
		w(storePkg+".locals", tWBox+".N", wat("55:14"), ConstructAccess),

		w(storePkg+".build", tWBox+".N", wat("61:14"), ConstructAccess),
		w(storePkg+".build", tWBox+".Inner", wat("61:20"), ConstructAccess),
		w(storePkg+".build", tWInner+".X", wat("61:33"), ConstructAccess),
		// Elided types; {2} is positional.
		w(storePkg+".points", tWInner+".X", wat("66:4"), ConstructAccess),
		w(storePkg+".points", tWInner+".X", wat("67:4"), ConstructAccess),

		// Outside targets are kept. DefaultClient is a pointer: content on the var.
		w(storePkg+".outside", "os.Args", wat("72:2"), AssignAccess),
		w(storePkg+".outside", "net/http.Client.Timeout", wat("73:2"), AssignAccess),
		w(storePkg+".outside", "net/http.DefaultClient", wat("73:2"), ContentAccess),
		w(storePkg+".outside", "net/http.Server.Addr", wat("74:22"), ConstructAccess),

		// for b.N = range xs: assigned each iteration.
		w("(*"+tWBox+").ranges", tWBox+".N", wat("78:6"), AssignAccess),

		// (*b.Ptr).X = 1: the same edges as b.Ptr.X = 1.
		w("(*"+tWBox+").derefs", tWInner+".X", wat("84:2"), AssignAccess),
		w("(*"+tWBox+").derefs", tWBox+".Ptr", wat("84:3"), ContentAccess),
		// wrapped.X = 1 through the embedded *Inner: content on the field holding the
		// pointer, then stop, so nothing on wrapped.
		w(storePkg+".promoted", tWInner+".X", wat("94:2"), AssignAccess),
		w(storePkg+".promoted", storePkg+".Wrapped.Inner", wat("94:2"), ContentAccess),

		// Writes in the closures fixture (store/closures.go). Func: the function the value is.
		wf(idClosures, tCRunner+".OnDone", cat("43:2"), AssignAccess, idClosures+"$6"),
		wf(idClosures, tCRunner+".OnDone", cat("44:2"), AssignAccess, idStart), // a method value
		wf(idClosures, tCRunner+".Run", cat("45:13"), ConstructAccess, idClosures+"$7"),
		wf(storePkg, storePkg+".hooks", cat("49:5"), ConstructAccess, storePkg+".hooks$1"),
		wf(storePkg, storePkg+".fallback", cat("49:12"), ConstructAccess, storePkg+".fallback$1"),
		wf(storePkg, storePkg+".nested", cat("53:5"), ConstructAccess, storePkg+".nested$1"),
		// 51: a blank var has no node, so no construct edge.

		// Writes in the reads fixture (store/reads.go).
		w("(*"+tRPart+").Bump", tRPart+".X", rat("13:25"), UpdateAccess),
		w(storePkg, storePkg+".limit", rat("46:5"), ConstructAccess),
		wf(storePkg, storePkg+".OnReady", rat("52:5"), ConstructAccess, storePkg+".OnReady$1"),
		w(idWritesR, tRPart+".X", rat("81:2"), AssignAccess),
		w(idWritesR, tRHolder+".Val", rat("81:2"), ContentAccess),
		w(idWritesR, tRPart+".X", rat("82:2"), AssignAccess),
		w(idWritesR, tRHolder+".Ptr", rat("82:2"), ContentAccess),
		w(idWritesR, tRPart+".X", rat("83:2"), AssignAccess),
		w(idWritesR, tRSub+".Ptr", rat("83:2"), ContentAccess),
		w(idWritesR, tRHolder+".Users", rat("84:2"), ContentAccess),
		w(idWritesR, tRHolder+".N", rat("85:2"), UpdateAccess),
		w(idWritesR, tRHolder+".N", rat("86:2"), UpdateAccess),
		w(idWritesR, tRHolder+".Items", rat("87:2"), UpdateAccess),
		w(idHands, tRHolder+".Items", rat("130:2"), AssignAccess),
		w(idHands, tRHolder+".Items", rat("131:13"), ConstructAccess),
	}

	// Reads and escapes have their own test.
	var got []Access
	for _, a := range g.Accesses {
		if a.Kind != ReadAccess && a.Kind != EscapeAccess {
			got = append(got, a)
		}
	}
	checkExact(t, "access", got, want, formatAccess)
}
