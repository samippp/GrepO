package query

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/samippp/grepo/internal/graph"
)

// calleesIndex: main calls Store.Get, Getter.Get (implemented by *Store, by
// Cache, and by Wrapper with a Get promoted from outside the repo), a local
// holding Get or handle, and a func value nothing resolves; it passes handle
// to http.HandleFunc and stores Get in a field. Get locks a mutex; handle
// calls itself and main$1, a closure that calls Get.
func calleesIndex() *Index {
	const (
		tWrapper = app + ".Wrapper"
		idOutGet = "(*example.com/dep.Inner).Get"
	)
	pg := &graph.PackageGraph{Packages: []graph.Package{{Path: app, Name: "main"}, {Path: store, Name: "store"}}}
	cg := &graph.CallGraph{
		Types: []graph.Type{{ID: tWrapper, Name: "Wrapper", Package: app, Pos: "wrap.go:3:6"}},
		Functions: []graph.Function{
			{ID: idMain, Name: "main", Package: app, Pos: "main.go:5:6"},
			{ID: idMain + "$1", Name: "main$1", Package: app, Pos: "main.go:30:7", Parent: idMain},
			{ID: idHandle, Name: "handle", Package: app, Pos: "main.go:20:6"},
			{ID: idGet, Name: "Get", Package: store, Owner: tStore, Pos: "store.go:10:17"},
			{ID: "(" + tCache + ").Get", Name: "Get", Package: store, Owner: tCache, Pos: "cache.go:4:16"},
			{ID: idIGet, Name: "Get", Package: store, Owner: tGetter, Pos: "store.go:3:2"},
		},
		External: []graph.Function{
			{ID: "(*sync.Mutex).Lock", Name: "Lock", Package: "sync", Owner: "sync.Mutex"},
			{ID: "net/http.HandleFunc", Name: "HandleFunc", Package: "net/http"},
			{ID: idOutGet, Name: "Get", Package: "example.com/dep", Owner: "example.com/dep.Inner"},
		},
		Calls: []graph.FunctionCall{
			{Caller: idMain, Callee: idGet, Pos: "main.go:6:7", CallKind: graph.StaticCall},
			{Caller: idMain, Callee: idIGet, Pos: "main.go:7:2", CallKind: graph.InterfaceCall},
			{Caller: idMain, Pos: "main.go:8:2", CallKind: graph.DynamicCall, Funcs: []string{idGet, idHandle}},
			{Caller: idMain, Pos: "main.go:9:2", CallKind: graph.DynamicCall, Through: tStore + ".onGet"},
			{Caller: idMain, Callee: "net/http.HandleFunc", Pos: "main.go:10:2", CallKind: graph.StaticCall,
				Args: []graph.Arg{{Expr: `"/"`, Type: "string"}, {Expr: "handle", Func: idHandle}}},
			{Caller: idMain, Callee: idMain + "$1", Pos: "main.go:30:2", CallKind: graph.StaticCall, Stmt: "go"},
			{Caller: idMain + "$1", Callee: idGet, Pos: "main.go:31:3", CallKind: graph.StaticCall},
			{Caller: idGet, Callee: "(*sync.Mutex).Lock", Pos: "store.go:11:2", CallKind: graph.StaticCall},
			{Caller: idHandle, Callee: idHandle, Pos: "main.go:21:2", CallKind: graph.StaticCall},
			{Caller: idHandle, Callee: idMain + "$1", Pos: "main.go:22:2", CallKind: graph.StaticCall},
		},
		Accesses: []graph.Access{
			{Accessor: idMain, Target: tStore + ".onGet", Pos: "main.go:11:2", Kind: graph.AssignAccess, Func: idGet},
		},
		Implementations: []graph.Implementation{
			{Type: tStore, Interface: tGetter, PointerOnly: true},
			{Type: tCache, Interface: tGetter},
			{Type: tWrapper, Interface: tGetter, Promoted: []string{idOutGet}},
		},
	}
	return New(pg, cg)
}

func TestCallees(t *testing.T) {
	ix := calleesIndex()
	main, _ := ix.Func(idMain, "")
	var got []string
	for _, s := range ix.Callees(main) {
		got = append(got, fmt.Sprintf("%s %s %s via=%s", s.Kind, s.Pos, s.Target, s.Via))
	}
	want := []string{
		"direct main.go:6:7 " + idGet + " via=",
		"interface main.go:7:2 " + idIGet + " via=" + tGetter,
		"local main.go:8:2 " + idGet + " via=", // a site per candidate
		"local main.go:8:2 " + idHandle + " via=",
		"dynamic main.go:9:2  via=",
		"direct main.go:10:2 net/http.HandleFunc via=",
		"value main.go:10:2 " + idHandle + " via=net/http.HandleFunc", // after the call it's passed to
		"value main.go:11:2 " + idGet + " via=" + tStore + ".onGet",
		"direct main.go:30:2 " + idMain + "$1 via=",
	}
	if !slices.Equal(got, want) {
		t.Errorf("Callees(main):\ngot  %q\nwant %q", got, want)
	}
}

// One method per implementing type, at its declaration; one promoted from
// outside the repo is at the type it's promoted to.
func TestImplementations(t *testing.T) {
	ix := calleesIndex()
	var got []string
	for _, s := range ix.Implementations(idIGet) {
		got = append(got, s.Pos+" "+s.Target+" via="+s.Via)
	}
	want := []string{
		"cache.go:4:16 (" + tCache + ").Get via=" + tGetter,
		"store.go:10:17 " + idGet + " via=" + tGetter,
		"wrap.go:3:6 (*example.com/dep.Inner).Get via=" + tGetter,
	}
	if !slices.Equal(got, want) {
		t.Errorf("Implementations(Getter.Get):\ngot  %q\nwant %q", got, want)
	}
	if s := ix.Implementations(idGet); s != nil {
		t.Errorf("Implementations of a concrete method = %v, want none", s)
	}
}

// renderTree renders nodes one per line, indented by level, with Seen/Recursive.
func renderTree(nodes []*Node, indent string) []string {
	var lines []string
	for _, n := range nodes {
		l := indent + string(n.Kind) + " " + n.Pos + " " + n.Target
		if n.Seen {
			l += " [see above]"
		}
		if n.Recursive {
			l += " [recursive]"
		}
		lines = append(lines, l)
		lines = append(lines, renderTree(n.Children, indent+"  ")...)
	}
	return lines
}

func TestCalleeTree(t *testing.T) {
	ix := calleesIndex()
	main, _ := ix.Func(idMain, "")
	lock := "direct store.go:11:2 (*sync.Mutex).Lock"
	for _, tc := range []struct {
		name   string
		depth  int
		direct bool
		want   []string
	}{
		{"one level", 1, false, []string{
			"direct main.go:6:7 " + idGet,
			"interface main.go:7:2 " + idIGet,
			"local main.go:8:2 " + idGet,
			"local main.go:8:2 " + idHandle,
			"dynamic main.go:9:2 ",
			"direct main.go:10:2 net/http.HandleFunc",
			"value main.go:10:2 " + idHandle,
			"value main.go:11:2 " + idGet,
			"direct main.go:30:2 " + idMain + "$1",
		}},
		{"all the way", 0, false, []string{
			"direct main.go:6:7 " + idGet,
			"  " + lock, // outside the repo: a leaf
			"interface main.go:7:2 " + idIGet,
			"  implementation cache.go:4:16 (" + tCache + ").Get",
			"  implementation store.go:10:17 " + idGet + " [see above]",
			"  implementation wrap.go:3:6 (*example.com/dep.Inner).Get",
			"local main.go:8:2 " + idGet + " [see above]",
			"local main.go:8:2 " + idHandle,
			"  direct main.go:21:2 " + idHandle + " [recursive]",
			"  direct main.go:22:2 " + idMain + "$1",
			"    direct main.go:31:3 " + idGet + " [see above]",
			"dynamic main.go:9:2 ",
			"direct main.go:10:2 net/http.HandleFunc",
			"value main.go:10:2 " + idHandle + " [see above]",
			"value main.go:11:2 " + idGet + " [see above]",
			"direct main.go:30:2 " + idMain + "$1 [see above]",
		}},
		{"two levels, direct only", 2, true, []string{
			"direct main.go:6:7 " + idGet,
			"  " + lock,
			"direct main.go:10:2 net/http.HandleFunc",
			"direct main.go:30:2 " + idMain + "$1",
			"  direct main.go:31:3 " + idGet + " [see above]",
		}},
	} {
		if got := renderTree(ix.CalleeTree(main, tc.depth, tc.direct), ""); !slices.Equal(got, tc.want) {
			t.Errorf("%s:\ngot\n%s\nwant\n%s", tc.name, strings.Join(got, "\n"), strings.Join(tc.want, "\n"))
		}
	}
}
