package graph

import (
	"fmt"
	"testing"
)

func TestCallGraphTypes(t *testing.T) {
	tRef := storePkg + ".Ref"
	g := buildFixtureCallGraph(t)

	st, mt := "store/store.go:", "model/kinds.go:"

	want := []Type{
		{ID: tStore, Name: "Store", Package: storePkg, Kind: StructType, Exported: true, Pos: st + "16:6", Fields: []Field{
			fld(tStore, "mu", "sync.Mutex", st+"17:2"),
			fld(tStore, "users", "map[int]"+tUser, st+"18:2"),
		}},
		{ID: tGetter, Name: "Getter", Package: storePkg, Kind: InterfaceType, Exported: true, Pos: st + "12:6"},
		{ID: tList, Name: "List", Package: mainPkg, Kind: StructType, Exported: true, Pos: "main.go:54:6",
			Fields: []Field{fld(tList, "items", "[]T", "main.go:54:26")}},
		{ID: tUser, Name: "User", Package: modelPkg, Kind: StructType, Exported: true, Pos: "model/model.go:4:6", Fields: []Field{
			fld(tUser, "ID", "int", "model/model.go:5:2"),
			fld(tUser, "Name", "string", "model/model.go:6:2"),
		}},
		// Embedded struct field: named after its type, Embedded set.
		{ID: tTagged, Name: "Tagged", Package: modelPkg, Kind: StructType, Exported: true, Pos: mt + "18:6", Fields: []Field{
			emb(tTagged, "User", tUser, mt+"19:2"),
			fld(tTagged, "Tag", "string", mt+"20:2"),
		}},
		// Alias: Target is what it names; no fields or methods of its own.
		{ID: tID, Name: "ID", Package: modelPkg, Kind: AliasType, Exported: true, Pos: mt + "6:6", Target: "int"},
		// Neither struct nor interface: Underlying says what it is.
		{ID: tUserID, Name: "UserID", Package: modelPkg, Kind: OtherType, Exported: true, Pos: mt + "8:6", Underlying: "int"},
		// Embedded interface: recorded in Embeds, its methods are not copied.
		{ID: tReadNamer, Name: "ReadNamer", Package: modelPkg, Kind: InterfaceType, Exported: true, Pos: mt + "13:6", Embeds: []string{"io.Reader"}},
		{ID: tCache, Name: "Cache", Package: storePkg, Kind: StructType, Exported: true, Pos: "store/cache.go:7:6"},
		{ID: tNamer, Name: "Namer", Package: modelPkg, Kind: InterfaceType, Exported: true, Pos: mt + "24:6"},
		{ID: tLabel, Name: "Label", Package: modelPkg, Kind: StructType, Exported: true, Pos: mt + "26:6"},
		{ID: tBadge, Name: "Badge", Package: modelPkg, Kind: StructType, Exported: true, Pos: mt + "30:6",
			Fields: []Field{emb(tBadge, "Label", tLabel, mt+"30:20")}},
		{ID: tAny, Name: "Any", Package: modelPkg, Kind: InterfaceType, Exported: true, Pos: mt + "33:6"},
		{ID: tSource, Name: "Source", Package: mainPkg, Kind: InterfaceType, Exported: true, Pos: "main.go:71:6"},
		{ID: tS, Name: "S", Package: storePkg, Kind: AliasType, Exported: true, Pos: "store/alias.go:4:6", Target: tStore},
		// Package-level result keeps its plain ID; the local ones can't collide with it.
		{ID: tResult, Name: "result", Package: mainPkg, Kind: StructType, Pos: "main.go:81:6",
			Fields: []Field{fld(tResult, "ok", "bool", "main.go:81:21")}},
		{ID: tParseResult, Name: "result", Package: mainPkg, Kind: StructType, Pos: "main.go:84:7",
			Fields: []Field{fld(tParseResult, "n", "int", "main.go:84:22")}},
		// Same name twice in one function: numbered in source order.
		{ID: tSaveResult1, Name: "result", Package: mainPkg, Kind: StructType, Pos: "main.go:89:8",
			Fields: []Field{fld(tSaveResult1, "err", "error", "main.go:89:23")}},
		{ID: tSaveResult2, Name: "result", Package: mainPkg, Kind: StructType, Pos: "main.go:91:8",
			Fields: []Field{fld(tSaveResult2, "msg", "string", "main.go:91:23")}},
		{ID: tLocalNamer, Name: "namer", Package: mainPkg, Kind: InterfaceType, Pos: "main.go:99:7"},
		// Inside a closure: the closure is its parent.
		{ID: tJob, Name: "job", Package: mainPkg, Kind: StructType, Pos: "main.go:108:8",
			Fields: []Field{emb(tJob, "Store", "*"+tStore, "main.go:108:27")}},
		// Uses wrap's T, but has no type params of its own: no [T] in the ID.
		{ID: tBox, Name: "box", Package: mainPkg, Kind: StructType, Pos: "main.go:114:7",
			Fields: []Field{fld(tBox, "v", "T", "main.go:114:19")}},

		{ID: tWInner, Name: "Inner", Package: storePkg, Kind: StructType, Exported: true, Pos: wat("12:6"),
			Fields: []Field{fld(tWInner, "X", "int", wat("12:20"))}},
		{ID: tWBox, Name: "Box", Package: storePkg, Kind: StructType, Exported: true, Pos: wat("14:6"), Fields: []Field{
			fld(tWBox, "N", "int", wat("15:2")),
			fld(tWBox, "Name", "string", wat("16:2")),
			fld(tWBox, "Items", "[]int", wat("17:2")),
			fld(tWBox, "Inner", tWInner, wat("18:2")),
			fld(tWBox, "Ptr", "*"+tWInner, wat("19:2")),
			fld(tWBox, "Buf", "[4]byte", wat("20:2")),
			fld(tWBox, "Users", "map[int]"+tUser, wat("21:2")),
			fld(tWBox, "Tags", "[]"+tTagged, wat("22:2")),
		}},
		{ID: storePkg + ".Wrapped", Name: "Wrapped", Package: storePkg, Kind: StructType, Exported: true, Pos: wat("89:6"),
			Fields: []Field{emb(storePkg+".Wrapped", "Inner", "*"+tWInner, wat("89:23"))}},

		{ID: tRPart, Name: "Part", Package: storePkg, Kind: StructType, Exported: true, Pos: rat("11:6"),
			Fields: []Field{fld(tRPart, "X", "int", rat("11:19"))}},
		{ID: tRBase, Name: "Base", Package: storePkg, Kind: StructType, Exported: true, Pos: rat("17:6"),
			Fields: []Field{fld(tRBase, "ID", "int", rat("17:19"))}},
		{ID: tRSub, Name: "Sub", Package: storePkg, Kind: StructType, Exported: true, Pos: rat("19:6"),
			Fields: []Field{fld(tRSub, "Ptr", "*"+tRPart, rat("19:18"))}},
		{ID: tRHolder, Name: "Holder", Package: storePkg, Kind: StructType, Exported: true, Pos: rat("21:6"), Fields: []Field{
			emb(tRHolder, "Base", tRBase, rat("22:2")),
			fld(tRHolder, "N", "int", rat("23:2")),
			fld(tRHolder, "Step", "int", rat("24:2")),
			fld(tRHolder, "Key", "int", rat("25:2")),
			fld(tRHolder, "Items", "[]int", rat("26:2")),
			fld(tRHolder, "Users", "map[int]string", rat("27:2")),
			fld(tRHolder, "Val", tRPart, rat("28:2")),
			fld(tRHolder, "Ptr", "*"+tRPart, rat("29:2")),
			fld(tRHolder, "Sub", tRSub, rat("30:2")),
			fld(tRHolder, "H", "net/http.Handler", rat("31:2")),
			fld(tRHolder, "OnErr", "func(error)", rat("32:2")),
			fld(tRHolder, "mu", "sync.Mutex", rat("33:2")),
		}},
		{ID: tRLocked, Name: "Locked", Package: storePkg, Kind: StructType, Exported: true, Pos: rat("39:6"),
			Fields: []Field{emb(tRLocked, "Mutex", "sync.Mutex", rat("39:26"))}},
		{ID: tRBag, Name: "Bag", Package: storePkg, Kind: StructType, Exported: true, Pos: rat("41:6"),
			Fields: []Field{fld(tRBag, "items", "[]T", rat("41:25"))}},
		{ID: storePkg + ".Slots", Name: "Slots", Package: storePkg, Kind: OtherType, Exported: true, Pos: rat("44:6"), Underlying: "[100]byte"},
		{ID: tROuter, Name: "Outer", Package: storePkg, Kind: StructType, Exported: true, Pos: rat("142:6"), Fields: []Field{
			fld(tROuter, "In", tRHolder, rat("143:2")),
			fld(tROuter, "L", tRLocked, rat("144:2")),
		}},

		{ID: tCRunner, Name: "Runner", Package: storePkg, Kind: StructType, Exported: true, Pos: cat("10:6"), Fields: []Field{
			fld(tCRunner, "OnDone", "func()", cat("11:2")),
			fld(tCRunner, "Run", "func() error", cat("12:2")),
		}},

		// The implementations fixture (model/impls.go, store/impls.go).
		{ID: tSealed, Name: "Sealed", Package: modelPkg, Kind: InterfaceType, Exported: true, Pos: "model/impls.go:8:6"},
		{ID: tToken, Name: "Token", Package: modelPkg, Kind: StructType, Exported: true, Pos: "model/impls.go:10:6"},
		{ID: tLookup, Name: "Lookup", Package: modelPkg, Kind: InterfaceType, Exported: true, Pos: "model/impls.go:15:6"},
		{ID: tFake, Name: "Fake", Package: storePkg, Kind: StructType, Exported: true, Pos: iat("6:6")},
		{ID: tSealer, Name: "Sealer", Package: storePkg, Kind: StructType, Exported: true, Pos: iat("11:6"),
			Fields: []Field{emb(tSealer, "Token", tToken, iat("11:27"))}},
		{ID: tFinder, Name: "Finder", Package: storePkg, Kind: StructType, Exported: true, Pos: iat("14:6")},
		{ID: tLogger, Name: "Logger", Package: storePkg, Kind: InterfaceType, Exported: true, Pos: iat("19:6")},
		{ID: tVarLog, Name: "varLog", Package: storePkg, Kind: StructType, Pos: iat("21:6")},
		{ID: tSliceLog, Name: "sliceLog", Package: storePkg, Kind: StructType, Pos: iat("25:6")},
		{ID: tMakersResult, Name: "result", Package: storePkg, Kind: StructType, Pos: iat("32:7")},
		{ID: tMaker, Name: "maker", Package: storePkg, Kind: InterfaceType, Pos: iat("33:7")},
		// The embedded field's type prints without its local ID: the open item on local type strings.
		{ID: tImpl, Name: "impl", Package: storePkg, Kind: StructType, Pos: iat("34:7"),
			Fields: []Field{emb(tImpl, "maker", storePkg+".maker", iat("34:20"))}},
		{ID: tOthersResult, Name: "result", Package: storePkg, Kind: StructType, Pos: iat("38:7")},
		{ID: tOther, Name: "other", Package: storePkg, Kind: InterfaceType, Pos: iat("39:7")},
		{ID: tStringish, Name: "Stringish", Package: modelPkg, Kind: InterfaceType, Exported: true, Pos: "model/impls.go:19:6", Embeds: []string{"~int"}},
		{ID: tWriter, Name: "Writer", Package: storePkg, Kind: InterfaceType, Exported: true, Pos: iat("43:6")},
		{ID: tRawWriter, Name: "rawWriter", Package: storePkg, Kind: StructType, Pos: iat("45:6")},
		{ID: tBufWriter, Name: "bufWriter", Package: storePkg, Kind: StructType, Pos: "store/outside.go:8:6"},
		{ID: tFailure, Name: "failure", Package: storePkg, Kind: StructType, Pos: "store/outside.go:15:6"},

		// The promoted methods fixture (store/promoted.go).
		{ID: tOpenCloser, Name: "OpenCloser", Package: storePkg, Kind: InterfaceType, Exported: true, Pos: pat("7:6")},
		{ID: tConn, Name: "conn", Package: storePkg, Kind: StructType, Pos: pat("12:6")},
		{ID: tWrapsConn, Name: "wrapsConn", Package: storePkg, Kind: StructType, Pos: pat("17:6"),
			Fields: []Field{emb(tWrapsConn, "conn", "*"+tConn, pat("17:25"))}},
		{ID: tDeeper, Name: "deeper", Package: storePkg, Kind: StructType, Pos: pat("21:6"),
			Fields: []Field{emb(tDeeper, "wrapsConn", tWrapsConn, pat("21:21"))}},
		{ID: tShadowed, Name: "shadowed", Package: storePkg, Kind: StructType, Pos: pat("24:6"),
			Fields: []Field{emb(tShadowed, "conn", "*"+tConn, pat("24:24"))}},
		{ID: tOSFile, Name: "osFile", Package: storePkg, Kind: StructType, Pos: pat("31:6"),
			Fields: []Field{emb(tOSFile, "File", "*os.File", pat("31:25"))}},

		// A const in a constraint (store/consts.go).
		{ID: storePkg + ".bounded[T]", Name: "bounded", Package: storePkg, Kind: StructType, Pos: "store/consts.go:13:6",
			Fields: []Field{fld(storePkg+".bounded[T]", "v", "T", "store/consts.go:13:45")}},

		// The type refs fixture (store/typerefs.go).
		{ID: tRef, Name: "Ref", Package: storePkg, Kind: StructType, Exported: true, Pos: tat("11:6"), Fields: []Field{
			fld(tRef, "Next", "*"+tRef, tat("12:2")),
			emb(tRef, "User", modelPkg+".User", tat("13:8")),
			fld(tRef, "Req", "*net/http.Request", tat("14:2")),
			fld(tRef, "refs", storePkg+".Refs", tat("15:2")),
		}},
		{ID: storePkg + ".Refs", Name: "Refs", Package: storePkg, Kind: OtherType, Exported: true, Pos: tat("18:6"), Underlying: "[]" + tRef},
		{ID: storePkg + ".RefAlias", Name: "RefAlias", Package: storePkg, Kind: AliasType, Exported: true, Pos: tat("20:6"), Target: tRef},
		{ID: storePkg + ".Ordered", Name: "Ordered", Package: storePkg, Kind: InterfaceType, Exported: true, Pos: tat("22:6"), Embeds: []string{"~int | ~string"}},
		{ID: storePkg + ".Sorted[T]", Name: "Sorted", Package: storePkg, Kind: StructType, Exported: true, Pos: tat("24:6"),
			Fields: []Field{fld(storePkg+".Sorted[T]", "items", "[]T", tat("24:32"))}},
		{ID: storePkg + ".Namer", Name: "Namer", Package: storePkg, Kind: InterfaceType, Exported: true, Pos: tat("26:6")},
		{ID: storePkg + ".refsIn.local#1", Name: "local", Package: storePkg, Kind: StructType, Pos: tat("45:7"),
			Fields: []Field{fld(storePkg+".refsIn.local#1", "r", tRef, tat("45:21"))}},
	}

	checkExact(t, "type", g.Types, want, func(ty Type) string { return fmt.Sprintf("%+v", ty) })
}

func TestCallGraphImplementations(t *testing.T) {
	g := buildFixtureCallGraph(t)

	want := []Implementation{
		// Value receiver: Cache and *Cache both implement Getter.
		{Type: tCache, Interface: tGetter},
		// Pointer receiver: only *Store implements Getter.
		{Type: tStore, Interface: tGetter, PointerOnly: true},
		{Type: tLabel, Interface: tNamer},
		// Badge has Name only through its embedded Label (a promoted method):
		// Promoted names the function a call through Namer can run.
		{Type: tBadge, Interface: tNamer, Promoted: []string{"(" + tLabel + ").Name"}},
		// Local interfaces take part like package-level ones.
		{Type: tLabel, Interface: tLocalNamer},
		{Type: tBadge, Interface: tLocalNamer, Promoted: []string{"(" + tLabel + ").Name"}},
		// A local struct implements through its embedded *Store, so its value does too.
		{Type: tJob, Interface: tGetter, Promoted: []string{idGet}},

		// Signatures must compare as the type checker does (model/impls.go, store/impls.go).
		// An unexported method belongs to its package: Token and Sealer (promoted) have
		// model's seal; Fake's seal is store's, so Fake isn't Sealed.
		{Type: tToken, Interface: tSealed},
		{Type: tSealer, Interface: tSealed, Promoted: []string{"(" + tToken + ").seal"}},
		// Promoted methods (store/promoted.go): Promoted is the function a call
		// through the interface runs, picked as Go selects. conn has no Open,
		// so only the types embedding it implement OpenCloser; deeper gets
		// both methods from two levels down; shadowed's own Close wins.
		{Type: tWrapsConn, Interface: tOpenCloser, Promoted: []string{idConnClose}},
		{Type: tDeeper, Interface: tOpenCloser, Promoted: []string{idConnClose, "(" + tWrapsConn + ").Open"}},
		{Type: tShadowed, Interface: tOpenCloser},
		// Outside methods are promoted too: osFile gets os.File's.
		{Type: tOSFile, Interface: tNamer, Promoted: []string{"(*os.File).Name"}},
		{Type: tOSFile, Interface: tLocalNamer, Promoted: []string{"(*os.File).Name"}},
		{Type: tOSFile, Interface: modelPkg + ".ReadNamer", Promoted: []string{"(*os.File).Name", "(*os.File).Read"}},
		{Type: tOSFile, Interface: "io.Writer", Promoted: []string{"(*os.File).Write"}},
		// Nothing promoted for impl: it gets Make from an embedded interface,
		// and a call through maker is already a call to maker's Make.
		// Param names and aliases don't matter: Find(n int) is Find(id ID).
		{Type: tFinder, Interface: tLookup},
		// ...string isn't []string: varLog is a Logger, sliceLog isn't.
		{Type: tVarLog, Interface: tLogger},
		// impl's Make returns makers' result, not others': a maker, not an other.
		{Type: tImpl, Interface: tMaker},
		// byte is uint8.
		{Type: tRawWriter, Interface: tWriter},
		// An interface from outside the repo, which flush calls through
		// (store/outside.go). bufWriter's Write has a pointer receiver.
		{Type: tBufWriter, Interface: "io.Writer", PointerOnly: true},
		// Built in, called through by main's describe.
		{Type: tFailure, Interface: "error"},
		// Nothing for Stringish: interfaces with type terms only constrain type
		// params, so they're left out, though UserID is in its type set.
		// Nothing for Any: empty interfaces are skipped.
		// Not listed: interfaces themselves, generic List[T] (no type args to check),
		// and interfaces outside the repo that no call goes through (UserID
		// implements fmt.Stringer, but fmt.Println takes any).
	}

	checkExact(t, "implementation", g.Implementations, want, func(im Implementation) string { return fmt.Sprintf("%+v", im) })
}

// fld is a field of the type owner; emb is an embedded one.
func tat(line string) string { return "store/typerefs.go:" + line }

func fld(owner, name, typ, pos string) Field {
	return Field{ID: owner + "." + name, Name: name, Type: typ, Pos: pos}
}

func emb(owner, name, typ, pos string) Field {
	f := fld(owner, name, typ, pos)
	f.Embedded = true
	return f
}
