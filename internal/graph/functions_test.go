package graph

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/samippp/grepo/internal/load"
)

func TestCallGraphFunctions(t *testing.T) {
	g := buildFixtureCallGraph(t)

	// Holder's unexported methods and the helpers in store/reads.go.
	holderMethod := func(name, line string, params, results []Param) Function {
		return Function{
			ID: "(*" + tRHolder + ")." + name, Name: name, Package: storePkg, Receiver: recv("r", "*"+tRHolder),
			Params: params, Results: results, Pos: rat(line), Owner: tRHolder,
		}
	}
	readsFunc := func(name, line string, params, results []Param) Function {
		return Function{ID: storePkg + "." + name, Name: name, Package: storePkg, Params: params, Results: results, Pos: rat(line)}
	}
	ints := ps("", "int")
	handler := ps("w", "net/http.ResponseWriter", "req", "*net/http.Request")

	want := []Function{
		{ID: idMain, Name: "main", Package: mainPkg, Pos: "main.go:15:6"},
		{ID: idGreet, Name: "greet", Package: mainPkg, Params: ps("name", "string"), Pos: "main.go:35:6"},
		{ID: idNew, Name: "New", Package: storePkg, Results: ps("", "*"+tStore), Exported: true, Pos: "store/store.go:21:6"},
		{ID: idGet, Name: "Get", Package: storePkg, Receiver: recv("s", "*"+tStore), Params: ps("id", "int"),
			Results: ps("", tUser), Exported: true, Pos: "store/store.go:23:17", Owner: tStore},
		{ID: idFirst, Name: "first", Package: mainPkg, Params: ps("xs", "[]T"), Results: ps("", "T"), Pos: "main.go:39:6"},
		{ID: idPair, Name: "pair", Package: mainPkg, Params: ps("k", "K", "v", "V"), Pos: "main.go:41:6"},
		{ID: idGenerics, Name: "generics", Package: mainPkg, Pos: "main.go:44:6"},
		{ID: idPush, Name: "Push", Package: mainPkg, Receiver: recv("l", "*"+tList), Params: ps("v", "T"),
			Exported: true, Pos: "main.go:56:19", Owner: tList},
		{ID: idUseList, Name: "useList", Package: mainPkg, Pos: "main.go:58:6"},
		// Several inits per package: file + position makes each ID unique (#6).
		{ID: idInit1, Name: "init", Package: mainPkg, Pos: "main.go:64:6"},
		{ID: idInit2, Name: "init", Package: mainPkg, Pos: "main.go:66:6"},
		{ID: idStoreInit, Name: "init", Package: storePkg, Pos: "store/store.go:29:6"},
		// A method named init is an ordinary method: normal ID.
		{ID: idListInit, Name: "init", Package: mainPkg, Receiver: recv("l", "*"+tList), Pos: "main.go:68:19", Owner: tList},
		// Value receiver on a non-struct type.
		{ID: "(" + tUserID + ").String", Name: "String", Package: modelPkg, Receiver: recv("u", tUserID),
			Results: ps("", "string"), Exported: true, Pos: "model/kinds.go:10:17", Owner: tUserID},
		// Interface methods: explicit ones only, owned by their interface, no receiver.
		{ID: "(" + tGetter + ").Get", Name: "Get", Package: storePkg, Params: ps("id", "int"), Results: ps("", tUser),
			Exported: true, Pos: "store/store.go:13:2", Owner: tGetter},
		{ID: "(" + tReadNamer + ").Name", Name: "Name", Package: modelPkg, Results: ps("", "string"),
			Exported: true, Pos: "model/kinds.go:15:2", Owner: tReadNamer},
		{ID: storePkg + ".load", Name: "load", Package: storePkg, Results: ps("", "int", "", "error"), Pos: "store/vars.go:13:6"},
		{ID: storePkg + ".register", Name: "register", Package: storePkg, Results: ps("", "bool"), Pos: "store/vars.go:19:6"},
		{ID: "(" + tCache + ").Get", Name: "Get", Package: storePkg, Receiver: recv("c", tCache), Params: ps("id", "int"),
			Results: ps("", tUser), Exported: true, Pos: "store/cache.go:9:16", Owner: tCache},
		{ID: "(" + tNamer + ").Name", Name: "Name", Package: modelPkg, Results: ps("", "string"),
			Exported: true, Pos: "model/kinds.go:24:23", Owner: tNamer},
		// Unnamed receiver: func (Label) Name().
		{ID: "(" + tLabel + ").Name", Name: "Name", Package: modelPkg, Receiver: recv("", tLabel), Results: ps("", "string"),
			Exported: true, Pos: "model/kinds.go:28:14", Owner: tLabel},
		// Method IDs use the owner's type ID. A generic interface method has no
		// constraint in its ID (not Source[T any]).
		{ID: "(" + tSource + ").Next", Name: "Next", Package: mainPkg, Results: ps("", "T"),
			Exported: true, Pos: "main.go:71:31", Owner: tSource},
		{ID: mainPkg + ".drain", Name: "drain", Package: mainPkg, Params: ps("s", mainPkg+".Source[int]"), Pos: "main.go:73:6"},
		// Receiver written as List[E]: the ID still uses the declaration's List[T].
		{ID: "(*" + tList + ").Len", Name: "Len", Package: mainPkg, Receiver: recv("l", "*"+mainPkg+".List[E]"),
			Results: ints, Exported: true, Pos: "main.go:75:19", Owner: tList},
		// Declared through the alias S: the ID and owner are Store's.
		{ID: "(*" + tStore + ").Reset", Name: "Reset", Package: storePkg, Receiver: recv("s", "*"+tS),
			Exported: true, Pos: "store/alias.go:6:13", Owner: tStore},
		{ID: mainPkg + ".describe", Name: "describe", Package: mainPkg, Params: ps("err", "error"), Results: ps("", "string"), Pos: "main.go:78:6"},
		{ID: mainPkg + ".parse", Name: "parse", Package: mainPkg, Pos: "main.go:83:6"},
		{ID: mainPkg + ".save", Name: "save", Package: mainPkg, Params: ps("ok", "bool"), Pos: "main.go:87:6"},
		{ID: mainPkg + ".announce", Name: "announce", Package: mainPkg, Params: ps("v", "any"), Pos: "main.go:98:6"},
		// A local interface's method: owned by the local type, ID built from its ID.
		{ID: "(" + tLocalNamer + ").Name", Name: "Name", Package: mainPkg, Results: ps("", "string"),
			Exported: true, Pos: "main.go:99:24", Owner: tLocalNamer},
		{ID: mainPkg + ".spawn", Name: "spawn", Package: mainPkg, Pos: "main.go:106:6"},
		{ID: mainPkg + ".wrap", Name: "wrap", Package: mainPkg, Params: ps("v", "T"), Pos: "main.go:113:6"},

		{ID: "(*" + tWBox + ").Writes", Name: "Writes", Package: storePkg, Receiver: recv("b", "*"+tWBox),
			Params: ps("o", "*"+tWBox, "src", "[]byte"), Exported: true, Pos: wat("30:15"), Owner: tWBox},
		{ID: storePkg + ".locals", Name: "locals", Package: storePkg, Pos: wat("52:6")},
		{ID: storePkg + ".build", Name: "build", Package: storePkg, Results: ps("", "*"+tWBox), Pos: wat("60:6")},
		{ID: storePkg + ".points", Name: "points", Package: storePkg, Results: ps("", "[]"+tWInner), Pos: wat("64:6")},
		{ID: storePkg + ".outside", Name: "outside", Package: storePkg, Results: ps("", "*net/http.Server"), Pos: wat("71:6")},
		{ID: "(*" + tWBox + ").ranges", Name: "ranges", Package: storePkg, Receiver: recv("b", "*"+tWBox),
			Params: ps("xs", "[]int"), Pos: wat("77:15"), Owner: tWBox},
		{ID: "(*" + tWBox + ").derefs", Name: "derefs", Package: storePkg, Receiver: recv("b", "*"+tWBox), Pos: wat("83:15"), Owner: tWBox},
		{ID: storePkg + ".promoted", Name: "promoted", Package: storePkg, Pos: wat("93:6")},

		{ID: "(*" + tRPart + ").Bump", Name: "Bump", Package: storePkg, Receiver: recv("p", "*"+tRPart),
			Exported: true, Pos: rat("13:16"), Owner: tRPart},
		{ID: "(" + tRPart + ").Value", Name: "Value", Package: storePkg, Receiver: recv("p", tRPart), Results: ints,
			Exported: true, Pos: rat("15:15"), Owner: tRPart},
		{ID: "(*" + tRHolder + ").Touch", Name: "Touch", Package: storePkg, Receiver: recv("r", "*"+tRHolder),
			Exported: true, Pos: rat("36:18"), Owner: tRHolder},
		readsFunc("use", "54:6", ps("n", "int"), nil),
		readsFunc("two", "56:6", ps("a", "int", "b", "int"), nil),
		readsFunc("sum", "58:6", ps("xs", "[]int"), ints),
		readsFunc("keep", "60:6", ps("p", "*"+tRPart), nil),
		holderMethod("values", "62:18", ps("xs", "[]int"), ints),
		holderMethod("writes", "80:18", ps("v", "string"), nil),
		holderMethod("addresses", "90:18", nil, nil),
		holderMethod("calls", "98:18", handler, nil),
		readsFunc("lock", "114:6", ps("l", "*"+tRLocked), nil),
		holderMethod("outsides", "118:18", ps("req", "*net/http.Request"), ints),
		holderMethod("hands", "127:18", ps("other", "*"+tRHolder, "ch", "chan []int"), ps("", "map[int]string")),
		readsFunc("bagLen", "139:6", ps("b", "*"+storePkg+".Bag[int]"), ints),
		readsFunc("sized", "157:6", ps("b", "[100]byte"), ps("", "[100]byte")),
		{ID: idNested, Name: "nested", Package: storePkg, Receiver: recv("o", "*"+tROuter), Pos: rat("147:17"), Owner: tROuter},

		// The closures fixture's declared functions (store/closures.go).
		{ID: idStart, Name: "start", Package: storePkg, Receiver: recv("r", "*"+tCRunner), Pos: cat("15:18"), Owner: tCRunner},
		{ID: "(*" + tCRunner + ").serve", Name: "serve", Package: storePkg, Receiver: recv("r", "*"+tCRunner),
			Params: handler, Pos: cat("17:18"), Owner: tCRunner},
		{ID: storePkg + ".handle", Name: "handle", Package: storePkg, Params: handler, Pos: cat("19:6")},
		{ID: storePkg + ".double", Name: "double", Package: storePkg, Params: ps("x", "T"), Results: ps("", "T"), Pos: cat("21:6")},
		{ID: storePkg + ".mapAll", Name: "mapAll", Package: storePkg, Params: ps("xs", "[]T", "f", "func(T) T"), Pos: cat("23:6")},
		{ID: idClosures, Name: "closures", Package: storePkg, Receiver: recv("r", "*"+tCRunner),
			Params: ps("xs", "[]string"), Pos: cat("29:18"), Owner: tCRunner},
		{ID: idLocals, Name: "locals", Package: storePkg, Receiver: recv("r", "*"+tCRunner),
			Params: ps("pick", "bool", "given", "func()"), Pos: cat("58:18"), Owner: tCRunner},
		{ID: storePkg + ".handleLater", Name: "handleLater", Package: storePkg, Pos: cat("75:6")},

		// The implementations fixture (model/impls.go, store/impls.go).
		{ID: "(" + tSealed + ").seal", Name: "seal", Package: modelPkg, Pos: "model/impls.go:8:24", Owner: tSealed},
		{ID: "(" + tToken + ").seal", Name: "seal", Package: modelPkg, Receiver: recv("", tToken), Pos: "model/impls.go:12:14", Owner: tToken},
		{ID: "(" + tLookup + ").Find", Name: "Find", Package: modelPkg, Params: ps("id", tID), Results: ps("", tUser),
			Exported: true, Pos: "model/impls.go:15:24", Owner: tLookup},
		{ID: "(" + tFake + ").seal", Name: "seal", Package: storePkg, Receiver: recv("", tFake), Pos: iat("8:13"), Owner: tFake},
		{ID: "(" + tFinder + ").Find", Name: "Find", Package: storePkg, Receiver: recv("", tFinder), Params: ps("n", "int"),
			Results: ps("", tUser), Exported: true, Pos: iat("16:15"), Owner: tFinder},
		// Variadic params print as their slice type.
		{ID: "(" + tLogger + ").Log", Name: "Log", Package: storePkg, Params: ps("args", "[]string"), Exported: true, Pos: iat("19:24"), Owner: tLogger},
		{ID: "(" + tVarLog + ").Log", Name: "Log", Package: storePkg, Receiver: recv("", tVarLog), Params: ps("xs", "[]string"),
			Exported: true, Pos: iat("23:15"), Owner: tVarLog},
		{ID: "(" + tSliceLog + ").Log", Name: "Log", Package: storePkg, Receiver: recv("", tSliceLog), Params: ps("xs", "[]string"),
			Exported: true, Pos: iat("27:17"), Owner: tSliceLog},
		{ID: storePkg + ".makers", Name: "makers", Package: storePkg, Pos: iat("31:6")},
		// Both results print as store.result: the open item on local type strings.
		{ID: "(" + tMaker + ").Make", Name: "Make", Package: storePkg, Results: ps("", storePkg+".result"),
			Exported: true, Pos: iat("33:24"), Owner: tMaker},
		{ID: storePkg + ".others", Name: "others", Package: storePkg, Pos: iat("37:6")},

		// The promoted methods fixture (store/promoted.go).
		{ID: "(" + tOpenCloser + ").Open", Name: "Open", Package: storePkg, Exported: true, Pos: pat("8:2"), Owner: tOpenCloser},
		{ID: "(" + tOpenCloser + ").Close", Name: "Close", Package: storePkg, Exported: true, Pos: pat("9:2"), Owner: tOpenCloser},
		{ID: idConnClose, Name: "Close", Package: storePkg, Receiver: recv("", "*"+tConn), Exported: true, Pos: pat("14:14"), Owner: tConn},
		{ID: "(" + tWrapsConn + ").Open", Name: "Open", Package: storePkg, Receiver: recv("", tWrapsConn), Exported: true, Pos: pat("19:18"), Owner: tWrapsConn},
		{ID: "(" + tShadowed + ").Open", Name: "Open", Package: storePkg, Receiver: recv("", tShadowed), Exported: true, Pos: pat("26:17"), Owner: tShadowed},
		{ID: "(" + tShadowed + ").Close", Name: "Close", Package: storePkg, Receiver: recv("", tShadowed), Exported: true, Pos: pat("27:17"), Owner: tShadowed},
		{ID: "(" + tOSFile + ").Open", Name: "Open", Package: storePkg, Receiver: recv("", tOSFile), Exported: true, Pos: pat("33:15"), Owner: tOSFile},
		{ID: storePkg + ".closeAll", Name: "closeAll", Package: storePkg, Params: ps("oc", tOpenCloser), Pos: pat("35:6")},
		{ID: "(" + tOther + ").Make", Name: "Make", Package: storePkg, Results: ps("", storePkg+".result"),
			Exported: true, Pos: iat("39:24"), Owner: tOther},
		{ID: "(" + tStringish + ").String", Name: "String", Package: modelPkg, Results: ps("", "string"),
			Exported: true, Pos: "model/impls.go:21:2", Owner: tStringish},
		{ID: "(" + tWriter + ").Write", Name: "Write", Package: storePkg, Params: ps("p", "[]uint8"), Exported: true, Pos: iat("43:24"), Owner: tWriter},
		{ID: "(" + tRawWriter + ").Write", Name: "Write", Package: storePkg, Receiver: recv("", tRawWriter), Params: ps("p", "[]byte"),
			Exported: true, Pos: iat("47:18"), Owner: tRawWriter},
		{ID: storePkg + ".tagLimit", Name: "tagLimit", Package: storePkg, Results: ps("", "int"), Pos: "store/consts.go:6:6"},
		// An io.Writer, and a call through io.Writer (store/outside.go).
		{ID: "(*" + tBufWriter + ").Write", Name: "Write", Package: storePkg, Receiver: recv("", "*"+tBufWriter),
			Params: ps("p", "[]byte"), Results: ps("", "int", "", "error"), Exported: true, Pos: "store/outside.go:10:19", Owner: tBufWriter},
		{ID: storePkg + ".flush", Name: "flush", Package: storePkg, Params: ps("w", "io.Writer"), Pos: "store/outside.go:12:6"},
		{ID: "(" + tFailure + ").Error", Name: "Error", Package: storePkg, Receiver: recv("", tFailure),
			Results: ps("", "string"), Exported: true, Pos: "store/outside.go:17:16", Owner: tFailure},
		// For callers --depth (store/tree.go).
		{ID: storePkg + ".walk", Name: "walk", Package: storePkg, Params: ps("n", "int"), Pos: "store/tree.go:4:6"},
		{ID: storePkg + ".visit", Name: "visit", Package: storePkg, Pos: "store/tree.go:12:6"},
		{ID: storePkg + ".leaf", Name: "leaf", Package: storePkg, Pos: "store/tree.go:14:6"},
	}

	// Closures have their own test.
	var got []Function
	for _, f := range g.Functions {
		if f.Parent == "" {
			got = append(got, f)
		}
	}
	checkExact(t, "function", got, want, formatFunction)
}

// Functions outside the repo that calls and func values name get a node
// without a position: interface methods owned by their interface, error's
// Error by no package. Only types are compared: the stdlib's param names
// aren't ours to pin.
func TestCallGraphExternal(t *testing.T) {
	g := buildFixtureCallGraph(t)
	method := func(owner, name string, recv *Param, params, results []Param) Function {
		ptr := ""
		if recv != nil && strings.HasPrefix(recv.Type, "*") {
			ptr = "*"
		}
		pkg := ""
		if i := strings.LastIndex(owner, "."); i >= 0 {
			pkg = owner[:i]
		}
		return Function{ID: "(" + ptr + owner + ")." + name, Name: name, Package: pkg, Receiver: recv,
			Params: params, Results: results, Exported: true, Owner: owner}
	}
	fn := func(pkg, name string, params, results []Param) Function {
		return Function{ID: pkg + "." + name, Name: name, Package: pkg, Params: params, Results: results, Exported: true}
	}
	handler := ps("", "net/http.ResponseWriter", "", "*net/http.Request")
	want := []Function{
		method("sync.Mutex", "Lock", recv("", "*sync.Mutex"), nil, nil),
		method("sync.Mutex", "Unlock", recv("", "*sync.Mutex"), nil, nil),
		method("error", "Error", nil, nil, ps("", "string")),
		method("io.Writer", "Write", nil, ps("", "[]byte"), ps("", "int", "", "error")),
		method("net/http.Handler", "ServeHTTP", nil, handler, nil),
		fn("fmt", "Println", ps("", "[]any"), ps("", "int", "", "error")),
		fn("net/http", "HandleFunc", ps("", "string", "", "func(net/http.ResponseWriter, *net/http.Request)"), nil),
		fn("sort", "Slice", ps("", "any", "", "func(i int, j int) bool"), nil),
	}

	typesOnly := func(f Function) string {
		var types []string
		for _, ps := range [][]Param{f.Params, f.Results} {
			var ts []string
			for _, p := range ps {
				ts = append(ts, p.Type)
			}
			types = append(types, strings.Join(ts, ", "))
		}
		recv := ""
		if f.Receiver != nil {
			recv = f.Receiver.Type
		}
		return fmt.Sprintf("%s {name=%s pkg=%s recv=%s params=(%s) results=(%s) exported=%v pos=%s owner=%s parent=%s}",
			f.ID, f.Name, f.Package, recv, types[0], types[1], f.Exported, f.Pos, f.Owner, f.Parent)
	}
	checkExact(t, "external function", g.External, want, typesOnly)
}

// Dependencies are the packages outside the repo that IDs name, wherever
// they come from: functions, fields (req.URL.Path's net/url) and vars
// (os.Args). The walk sees the same objects through go/packages, whose
// dependencies come from export data and import less, so it finds the same.
func TestCallGraphDependencies(t *testing.T) {
	g := buildFixtureCallGraph(t)
	want := []Dependency{
		{"fmt", "fmt"}, {"io", "io"}, {"net/http", "http"}, {"net/url", "url"},
		{"os", "os"}, {"sort", "sort"}, {"sync", "sync"},
	}
	if !slices.Equal(g.Dependencies, want) {
		t.Errorf("dependencies = %v, want %v", g.Dependencies, want)
	}

	res, _, err := load.GoPackages(t.Context(), "testdata/simple", slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	viaPackages, _, err := BuildCallGraph(res)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(viaPackages.Dependencies, want) {
		t.Errorf("through go/packages, dependencies = %v, want %v", viaPackages.Dependencies, want)
	}
}

func TestCallGraphClosures(t *testing.T) {
	g := buildFixtureCallGraph(t)

	// closure is a closure node: no receiver, owner or export, even inside methods.
	// The fixture's closures are in store, except main.go's.
	closure := func(parent string, n int, pos string, params, results []Param) Function {
		id := fmt.Sprintf("%s$%d", parent, n)
		pkg := storePkg
		if strings.HasPrefix(pos, "main.go:") {
			pkg = mainPkg
		}
		return Function{
			ID: id, Name: id[strings.LastIndex(id, ".")+1:], Package: pkg,
			Params: params, Results: results, Pos: pos, Parent: parent,
		}
	}
	c1 := idClosures + "$1"

	want := []Function{
		// Existing fixture code.
		closure(idMain, 1, "main.go:25:2", nil, nil),
		closure(mainPkg+".spawn", 1, "main.go:107:5", nil, nil), // holds the local type job
		closure(storePkg+".OnReady", 1, rat("52:15"), nil, nil),
		closure(idValues, 1, rat("76:2"), nil, nil),
		closure(storePkg+".sized", 1, rat("159:6"), ps("d", "[100]byte"), nil),

		// In a generic function.
		closure(storePkg+".mapAll", 1, cat("25:3"), nil, nil),
		// In a method, numbered in source order; nested ones under their parent.
		closure(idClosures, 1, cat("30:2"), nil, nil),
		closure(c1, 1, cat("31:3"), nil, nil),
		closure(idClosures, 2, cat("33:9"), nil, nil),
		closure(idClosures, 3, cat("37:5"), nil, nil),
		closure(idClosures, 4, cat("38:8"), nil, nil),
		closure(idClosures, 5, cat("39:17"), ps("i", "int", "j", "int"), ps("", "bool")),
		closure(idClosures, 6, cat("43:13"), nil, nil),
		closure(idClosures, 7, cat("45:18"), nil, ps("", "error")),
		// Package level: under the var each value initializes.
		closure(storePkg+".hooks", 1, cat("49:23"), nil, nil),
		closure(storePkg+".fallback", 1, cat("49:34"), nil, nil),
		closure(storePkg+".nested", 1, cat("53:14"), nil, nil),
		closure(storePkg+".nested$1", 1, cat("54:2"), nil, nil),
		// Locals holding functions.
		closure(idLocals, 1, cat("61:7"), nil, nil),
		closure(idLocals, 2, cat("64:2"), nil, nil),
	}
	// A blank var has no node: its closure is the package's, named after _.
	blank := closure(storePkg+"._", 1, cat("51:9"), nil, ps("", "int"))
	blank.Parent = storePkg
	want = append(want, blank)

	var got []Function
	for _, f := range g.Functions {
		if f.Parent != "" {
			got = append(got, f)
		}
	}
	checkExact(t, "closure", got, want, formatFunction)
}

// ps builds params from name, type pairs: ps("", "int") is one unnamed int.
func ps(nameTypes ...string) []Param {
	var out []Param
	for i := 0; i+1 < len(nameTypes); i += 2 {
		out = append(out, Param{Name: nameTypes[i], Type: nameTypes[i+1]})
	}
	return out
}

func recv(name, typ string) *Param { return &Param{Name: name, Type: typ} }

func formatFunction(f Function) string {
	recv := "<nil>"
	if f.Receiver != nil {
		recv = fmt.Sprint(*f.Receiver)
	}
	return fmt.Sprintf("%s {name=%s pkg=%s recv=%s params=%v results=%v exported=%v pos=%s owner=%s parent=%s}",
		f.ID, f.Name, f.Package, recv, f.Params, f.Results, f.Exported, f.Pos, f.Owner, f.Parent)
}

// TestFuncIDMatchesNodes checks that FuncID names the node the graph has for
// every declared function, init functions included: there are several per
// package, and their IDs come from the walk.
func TestFuncIDMatchesNodes(t *testing.T) {
	res, g := loadFixture(t), buildFixtureCallGraph(t)
	byPos := map[string]string{}
	for _, f := range g.Functions {
		byPos[f.Pos] = f.ID
	}
	inits := 0
	for _, pkg := range res.Packages {
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}
				fn := pkg.TypesInfo.Defs[fd.Name].(*types.Func)
				pos := FormatPos(res.Dir, pkg.Fset.Position(fn.Pos()))
				if got, want := g.FuncID(fn), byPos[pos]; got != want {
					t.Errorf("FuncID(%s at %s) = %q, want %q", fn.Name(), pos, got, want)
				}
				if fd.Name.Name == "init" && fd.Recv == nil {
					inits++
				}
			}
		}
	}
	if inits != 3 {
		t.Errorf("checked %d init functions, want the fixture's 3", inits)
	}
}

func TestSplitPos(t *testing.T) {
	for _, tc := range []struct {
		pos       string
		file      string
		line, col int
	}{
		{"store/store.go:29:6", "store/store.go", 29, 6},
		{"c:/repo/main.go:7:2", "c:/repo/main.go", 7, 2}, // only the last two colons split
		{"c:/repo/main.go:7", "c:/repo/main.go", 7, 0},   // no column
		{"main.go", "main.go", 0, 0},
	} {
		if file, line, col := SplitPos(tc.pos); file != tc.file || line != tc.line || col != tc.col {
			t.Errorf("SplitPos(%q) = %q, %d, %d; want %q, %d, %d", tc.pos, file, line, col, tc.file, tc.line, tc.col)
		}
	}
}

// FormatPos and SplitPos round-trip.
func TestFormatPos(t *testing.T) {
	pos := FormatPos("/repo", token.Position{Filename: "/repo/store/store.go", Line: 23, Column: 17})
	if pos != "store/store.go:23:17" {
		t.Fatalf("FormatPos = %q, want store/store.go:23:17", pos)
	}
	if file, line, col := SplitPos(pos); file != "store/store.go" || line != 23 || col != 17 {
		t.Errorf("SplitPos(%q) = %q, %d, %d", pos, file, line, col)
	}
}
