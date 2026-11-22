package graph

import (
	"fmt"
	"testing"
)

func TestCallGraphCalls(t *testing.T) {
	g := buildFixtureCallGraph(t)

	want := []FunctionCall{
		// store.New(): function in another package.
		{Caller: idMain, Callee: idNew, Pos: "main.go:16:7", CallKind: StaticCall},
		// s.Get(1) where s is *Store: method on a concrete type.
		{Caller: idMain, Callee: idGet, Pos: "main.go:17:7", CallKind: StaticCall,
			Args: []Arg{{Expr: "1", Type: "int"}}},
		// g.Get(2) where g is store.Getter: points at the interface method.
		{Caller: idMain, Callee: "(" + storePkg + ".Getter).Get", Pos: "main.go:20:2", CallKind: InterfaceCall,
			Args: []Arg{{Expr: "2", Type: "int"}}},
		// f(u.Name) where f := greet: a function value, target unknown.
		{Caller: idMain, Callee: "", Pos: "main.go:23:2", CallKind: DynamicCall,
			Args: []Arg{{Expr: "u.Name", Type: "string"}}, Funcs: []string{idGreet}},
		// func(){...}(): a static call to the closure, and greet("closure") inside it belongs to it.
		{Caller: idMain, Callee: idMain + "$1", Pos: "main.go:25:2", CallKind: StaticCall},
		{Caller: idMain + "$1", Callee: idGreet, Pos: "main.go:26:3", CallKind: StaticCall,
			Args: []Arg{{Expr: `"closure"`, Type: "string"}}},
		// Nothing for len(u.Name) (built-in) or int64(n) (conversion).
		// fmt.Println(u): outside the repo, but kept.
		{Caller: idMain, Callee: "fmt.Println", Pos: "main.go:32:2", CallKind: StaticCall,
			Args: []Arg{{Expr: "u", Type: "example.com/simple/model.User"}}},
		{Caller: idGreet, Callee: "fmt.Println", Pos: "main.go:36:2", CallKind: StaticCall,
			Args: []Arg{{Expr: `"hello"`, Type: "string"}, {Expr: "name", Type: "string"}}},
		// s.mu.Lock(): method reached through a struct field.
		{Caller: idGet, Callee: "(*sync.Mutex).Lock", Pos: "store/store.go:24:2", CallKind: StaticCall},
		// defer s.mu.Unlock(): deferred calls count too.
		{Caller: idGet, Callee: "(*sync.Mutex).Unlock", Pos: "store/store.go:25:8", CallKind: StaticCall, Stmt: "defer"},
		// first[int](xs): explicit type arg (IndexExpr), target known (#4).
		{Caller: idGenerics, Callee: idFirst, Pos: "main.go:46:2", CallKind: StaticCall,
			Args: []Arg{{Expr: "xs", Type: "[]int"}}},
		// pair[string, int](...): two type args (IndexListExpr) (#4).
		{Caller: idGenerics, Callee: idPair, Pos: "main.go:47:2", CallKind: StaticCall,
			Args: []Arg{{Expr: `"a"`, Type: "string"}, {Expr: "1", Type: "int"}}},
		// handlers[0](...): also an IndexExpr, but a func value, so dynamic.
		{Caller: idGenerics, Callee: "", Pos: "main.go:50:2", CallKind: DynamicCall,
			Args: []Arg{{Expr: `"indexed"`, Type: "string"}}},
		// l.Push(1) on List[int]: callee must be the declared List[T] method (#5).
		{Caller: idUseList, Callee: idPush, Pos: "main.go:60:2", CallKind: StaticCall,
			Args: []Arg{{Expr: "1", Type: "int"}}},
		// Calls from each init are attributed to that init (#6).
		{Caller: idInit1, Callee: idGreet, Pos: "main.go:64:15", CallKind: StaticCall,
			Args: []Arg{{Expr: `"init 1"`, Type: "string"}}},
		{Caller: idInit2, Callee: idGreet, Pos: "main.go:66:15", CallKind: StaticCall,
			Args: []Arg{{Expr: `"init 2"`, Type: "string"}}},
		// Package-level calls: the caller is the package itself (#7).
		{Caller: storePkg, Callee: idNew, Pos: "store/vars.go:4:20", CallKind: StaticCall},
		{Caller: storePkg, Callee: storePkg + ".load", Pos: "store/vars.go:11:18", CallKind: StaticCall},
		// var _ = register() has no variable node, but the call is still recorded.
		{Caller: storePkg, Callee: storePkg + ".register", Pos: "store/vars.go:17:9", CallKind: StaticCall},
		// Nothing for var _ Getter = (*Store)(nil): a conversion, not a call.
		// s.Next() on a generic interface: the callee matches the method node's ID.
		{Caller: mainPkg + ".drain", Callee: "(" + tSource + ").Next", Pos: "main.go:73:29", CallKind: InterfaceCall},
		// err.Error(): error is built in, with no package path in its ID.
		{Caller: mainPkg + ".describe", Callee: "(error).Error", Pos: "main.go:78:42", CallKind: InterfaceCall},
		// n.Name() through a local interface: the callee has a node now.
		{Caller: mainPkg + ".announce", Callee: "(" + tLocalNamer + ").Name", Pos: "main.go:101:3", CallKind: InterfaceCall},
		// w.Write(nil) through io.Writer, from outside the repo: no node, but
		// bufWriter's implementation of it is recorded (store/outside.go).
		{Caller: storePkg + ".flush", Callee: "(io.Writer).Write", Pos: "store/outside.go:12:27", CallKind: InterfaceCall,
			Args: []Arg{{Expr: "nil", Type: "untyped nil"}}},

		// The reads fixture's calls (store/reads.go). Their Through is in TestCallGraphThrough.
		{Caller: idValues, Callee: storePkg + ".use", Pos: rat("64:2"), CallKind: StaticCall, Args: []Arg{{Expr: "r.N", Type: "int"}}},
		{Caller: idValues, Callee: storePkg + ".two", Pos: rat("75:2"), CallKind: StaticCall, Args: []Arg{{Expr: "r.N", Type: "int"}, {Expr: "r.N", Type: "int"}}},
		{Caller: idValues, Callee: idValues + "$1", Pos: rat("76:2"), CallKind: StaticCall}, // the func literal called where it's written
		{Caller: idCalls, Callee: "(*" + tRPart + ").Bump", Pos: rat("99:2"), CallKind: StaticCall},
		{Caller: idCalls, Callee: "(*" + tRPart + ").Bump", Pos: rat("100:2"), CallKind: StaticCall},
		{Caller: idCalls, Callee: "(" + tRPart + ").Value", Pos: rat("101:2"), CallKind: StaticCall},
		{Caller: idCalls, Callee: "(net/http.Handler).ServeHTTP", Pos: rat("102:2"), CallKind: InterfaceCall,
			Args: []Arg{{Expr: "w", Type: "net/http.ResponseWriter"}, {Expr: "req", Type: "*net/http.Request"}}},
		{Caller: idCalls, Callee: "", Pos: rat("103:2"), CallKind: DynamicCall, Args: []Arg{{Expr: "nil", Type: "untyped nil"}}},
		{Caller: idCalls, Callee: "(*sync.Mutex).Lock", Pos: rat("104:2"), CallKind: StaticCall},
		{Caller: idCalls, Callee: "(*" + tRHolder + ").Touch", Pos: rat("105:2"), CallKind: StaticCall},
		{Caller: idCalls, Callee: "(*" + tRHolder + ").Touch", Pos: rat("106:2"), CallKind: StaticCall},
		{Caller: idCalls, Callee: "", Pos: rat("107:2"), CallKind: DynamicCall},
		{Caller: idCalls, Callee: "", Pos: rat("110:2"), CallKind: DynamicCall, Funcs: []string{"(*" + tRPart + ").Bump"}},
		{Caller: idCalls, Callee: "", Pos: rat("111:2"), CallKind: DynamicCall, Funcs: []string{"(*" + tRPart + ").Bump"}},
		{Caller: storePkg + ".lock", Callee: "(*sync.Mutex).Lock", Pos: rat("115:2"), CallKind: StaticCall},
		{Caller: idHands, Callee: storePkg + ".sum", Pos: rat("128:6"), CallKind: StaticCall, Args: []Arg{{Expr: "r.Items", Type: "[]int"}}},
		{Caller: idHands, Callee: storePkg + ".use", Pos: rat("128:2"), CallKind: StaticCall, Args: []Arg{{Expr: "sum(r.Items)", Type: "int"}}},
		{Caller: idHands, Callee: storePkg + ".keep", Pos: rat("133:2"), CallKind: StaticCall, Args: []Arg{{Expr: "r.Ptr", Type: "*" + tRPart}}},
		{Caller: idHands, Callee: storePkg + ".sum", Pos: rat("134:6"), CallKind: StaticCall, Args: []Arg{{Expr: "r.Items[1:]", Type: "[]int"}}},
		{Caller: idHands, Callee: storePkg + ".use", Pos: rat("134:2"), CallKind: StaticCall, Args: []Arg{{Expr: "sum(r.Items[1:])", Type: "int"}}},
		{Caller: idNested, Callee: "(*" + tRPart + ").Bump", Pos: rat("149:2"), CallKind: StaticCall, Args: []Arg{}},
		{Caller: idNested, Callee: "(*sync.Mutex).Lock", Pos: rat("150:2"), CallKind: StaticCall, Args: []Arg{}},

		// go func(){...}() in spawn.
		{Caller: mainPkg + ".spawn", Callee: mainPkg + ".spawn$1", Pos: "main.go:107:5", CallKind: StaticCall, Stmt: "go"},

		// The closures fixture (store/closures.go).
		{Caller: storePkg + ".mapAll", Callee: storePkg + ".mapAll$1", Pos: cat("25:3"), CallKind: StaticCall},
		{Caller: storePkg + ".mapAll$1", Callee: "", Pos: cat("25:20"), CallKind: DynamicCall, Args: []Arg{{Expr: "xs[i]", Type: "T"}}},
		{Caller: idClosures, Callee: idClosures + "$1", Pos: cat("30:2"), CallKind: StaticCall},
		{Caller: idClosures + "$1", Callee: idClosures + "$1$1", Pos: cat("31:3"), CallKind: StaticCall},
		{Caller: idClosures, Callee: "", Pos: cat("34:2"), CallKind: DynamicCall, Funcs: []string{idClosures + "$2"}}, // inc(): through a local
		{Caller: idClosures, Callee: idStart, Pos: cat("35:5"), CallKind: StaticCall, Stmt: "go"},
		{Caller: idClosures, Callee: idStart, Pos: cat("36:8"), CallKind: StaticCall, Stmt: "defer"},
		{Caller: idClosures, Callee: idClosures + "$3", Pos: cat("37:5"), CallKind: StaticCall, Stmt: "go"},
		{Caller: idClosures + "$3", Callee: idStart, Pos: cat("37:14"), CallKind: StaticCall},
		{Caller: idClosures, Callee: idClosures + "$4", Pos: cat("38:8"), CallKind: StaticCall, Stmt: "defer"},
		// Func: the function an argument is: a closure, a declared function, a method value, a generic instance.
		{Caller: idClosures, Callee: "sort.Slice", Pos: cat("39:2"), CallKind: StaticCall, Args: []Arg{
			{Expr: "xs", Type: "[]string"},
			{Expr: "(func(i, j int) bool literal)", Type: "func(i int, j int) bool", Func: idClosures + "$5"}}},
		{Caller: idClosures, Callee: "net/http.HandleFunc", Pos: cat("40:2"), CallKind: StaticCall, Args: []Arg{
			{Expr: `"/a"`, Type: "string"},
			{Expr: "handle", Type: "func(w net/http.ResponseWriter, req *net/http.Request)", Func: storePkg + ".handle"}}},
		{Caller: idClosures, Callee: "net/http.HandleFunc", Pos: cat("41:2"), CallKind: StaticCall, Args: []Arg{
			{Expr: `"/b"`, Type: "string"},
			{Expr: "r.serve", Type: "func(w net/http.ResponseWriter, req *net/http.Request)", Func: "(*" + tCRunner + ").serve"}}},
		{Caller: idClosures, Callee: storePkg + ".mapAll", Pos: cat("42:2"), CallKind: StaticCall, Args: []Arg{
			{Expr: "xs", Type: "[]string"},
			{Expr: "double[string]", Type: "func(x string) string", Func: storePkg + ".double"}}},
		{Caller: idClosures, Callee: "", Pos: cat("46:2"), CallKind: DynamicCall}, // r.OnDone(): Through, not a target
		// Through locals: Funcs is every function assigned to the local, wherever it's called from.
		{Caller: idLocals, Callee: "", Pos: cat("63:2"), CallKind: DynamicCall, Funcs: []string{idLocals + "$1", idStart}},
		{Caller: idLocals, Callee: idLocals + "$2", Pos: cat("64:2"), CallKind: StaticCall},
		{Caller: idLocals + "$2", Callee: "", Pos: cat("64:11"), CallKind: DynamicCall, Funcs: []string{idLocals + "$1", idStart}},
		// Assigned after the call, in the loop; then a param nothing assigns.
		{Caller: idLocals, Callee: "", Pos: cat("68:4"), CallKind: DynamicCall, Funcs: []string{storePkg + ".handleLater"}},
		{Caller: idLocals, Callee: "", Pos: cat("72:2"), CallKind: DynamicCall},
		// The promoted methods fixture: a call through OpenCloser.
		{Caller: storePkg + ".closeAll", Callee: "(" + tOpenCloser + ").Close", Pos: pat("35:32"), CallKind: InterfaceCall},
		// Package level: the package calls the blank var's closure; nested's closure calls its own.
		{Caller: storePkg, Callee: storePkg + "._$1", Pos: cat("51:9"), CallKind: StaticCall},
		{Caller: storePkg + ".nested$1", Callee: storePkg + ".nested$1$1", Pos: cat("54:2"), CallKind: StaticCall},
	}

	checkExact(t, "call", g.Calls, want, formatCall)
}

func TestCallGraphThrough(t *testing.T) {
	g := buildFixtureCallGraph(t)

	type site struct{ caller, callee, pos string }

	// Through: the field or package var a call is made on, or takes its func from.
	// Calls on locals, params and results have none.
	want := map[site]string{
		{idGet, "(*sync.Mutex).Lock", "store/store.go:24:2"}:   tStore + ".mu",
		{idGet, "(*sync.Mutex).Unlock", "store/store.go:25:8"}: tStore + ".mu",

		{idCalls, "(*" + tRPart + ").Bump", rat("99:2")}:         tRHolder + ".Ptr",   // pointer field
		{idCalls, "(*" + tRPart + ").Bump", rat("100:2")}:        tRHolder + ".Val",   // value field, pointer method
		{idCalls, "(" + tRPart + ").Value", rat("101:2")}:        tRHolder + ".Val",   // value method
		{idCalls, "(net/http.Handler).ServeHTTP", rat("102:2")}:  tRHolder + ".H",     // interface field
		{idCalls, "", rat("103:2")}:                              tRHolder + ".OnErr", // the func stored in OnErr
		{idCalls, "(*sync.Mutex).Lock", rat("104:2")}:            tRHolder + ".mu",
		{idCalls, "(*" + tRHolder + ").Touch", rat("106:2")}:     storePkg + ".shared",  // a package var
		{idCalls, "", rat("107:2")}:                              storePkg + ".OnReady", // a package var holding a func
		{storePkg + ".lock", "(*sync.Mutex).Lock", rat("115:2")}: tRLocked + ".Mutex",   // promoted: the embedded field
		{idNested, "(*" + tRPart + ").Bump", rat("149:2")}:       tRHolder + ".Val",     // the innermost field
		{idNested, "(*sync.Mutex).Lock", rat("150:2")}:           tRLocked + ".Mutex",
		{idClosures, "", "store/closures.go:46:2"}:               tCRunner + ".OnDone", // a func field
		// None for r.Touch() (a param) or f() and g() (method values in locals).
	}

	var got []FunctionCall
	for _, c := range g.Calls {
		if c.Through != "" {
			got = append(got, c)
		}
	}
	var wantCalls []FunctionCall
	for k, through := range want {
		wantCalls = append(wantCalls, FunctionCall{Caller: k.caller, Callee: k.callee, Pos: k.pos, Through: through})
	}
	checkExact(t, "through", got, wantCalls, func(c FunctionCall) string {
		return fmt.Sprintf("%s -> %q at %s: %s", c.Caller, c.Callee, c.Pos, c.Through)
	})
}

func formatCall(c FunctionCall) string {
	s := fmt.Sprintf("%s -> %q at %s (%s) args=%v", c.Caller, c.Callee, c.Pos, c.CallKind, c.Args)
	if c.Stmt != "" {
		s += " stmt=" + c.Stmt
	}
	if c.Funcs != nil {
		s += fmt.Sprintf(" funcs=%v", c.Funcs)
	}
	return s
}
