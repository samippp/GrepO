package query

import (
	"fmt"
	"strings"
	"testing"

	"github.com/samippp/grepo/internal/graph"
)

const fUsers = tStore + ".users"

// findIndex is a repo with a declaration of each kind: Store.Get is called
// twice directly and once through Getter, which Store and Cache implement.
func findIndex() *Index {
	pg := &graph.PackageGraph{Packages: []graph.Package{{Path: app, Name: "main"}, {Path: store, Name: "store"}}}
	cg := &graph.CallGraph{
		Types: []graph.Type{
			{ID: tGetter, Name: "Getter", Package: store, Kind: graph.InterfaceType, Pos: "store/store.go:5:6"},
			{ID: tStore, Name: "Store", Package: store, Kind: graph.StructType, Pos: "store/store.go:9:6", Fields: []graph.Field{
				{ID: fUsers, Name: "users", Type: "map[int]string", Pos: "store/store.go:10:2"},
				{ID: tStore + ".getCount", Name: "getCount", Type: "int", Pos: "store/store.go:11:2"},
				{ID: tStore + ".hits", Name: "hits", Type: "int", Pos: "store/store.go:12:2"},
				{ID: tStore + ".label", Name: "label", Type: "string", Pos: "store/store.go:13:2"},
				{ID: tStore + ".created", Name: "created", Type: "int64", Pos: "store/store.go:15:2"},
			}},
			{ID: tCache, Name: "Cache", Package: store, Kind: graph.StructType, Pos: "store/cache.go:3:6", Fields: []graph.Field{
				{ID: tCache + ".hits", Name: "hits", Type: "int", Pos: "store/cache.go:3:20"},
			}},
			{ID: store + ".ID", Name: "ID", Package: store, Kind: graph.AliasType, Pos: "store/store.go:3:6"},
			{ID: store + ".S", Name: "S", Package: store, Kind: graph.AliasType, Target: tStore, Pos: "store/alias.go:3:6"},
			{ID: store + ".Count", Name: "Count", Package: store, Kind: graph.OtherType, Pos: "store/store.go:4:6"},
			// A local type, its field and its method: not declarations find lists.
			{ID: app + ".run.getter#1", Name: "getter", Package: app, Kind: graph.StructType, Pos: "main.go:30:7", Fields: []graph.Field{
				{ID: app + ".run.getter#1.get", Name: "get", Pos: "main.go:30:21"},
			}},
		},
		Variables: []graph.Variable{
			{ID: store + ".cache", Name: "cache", Package: store, Pos: "store/cache.go:8:5"},
			{ID: store + ".maxGets", Name: "maxGets", Package: store, Const: true, Pos: "store/store.go:2:7"},
		},
		Functions: []graph.Function{
			{ID: idGet, Name: "Get", Package: store, Owner: tStore, Pos: "store/store.go:14:17"},
			{ID: "(" + tCache + ").Get", Name: "Get", Package: store, Owner: tCache, Pos: "store/cache.go:5:16"},
			{ID: "(" + tGetter + ").Get", Name: "Get", Package: store, Owner: tGetter, Pos: "store/store.go:6:2"},
			{ID: store + ".Get", Name: "Get", Package: store, Pos: "store/get.go:3:6"},
			{ID: store + ".getAll", Name: "getAll", Package: store, Pos: "store/get.go:10:6"},
			{ID: store + ".New", Name: "New", Package: store, Pos: "store/store.go:19:6"},
			{ID: idMain, Name: "main", Package: app, Pos: "main.go:3:6"},
			{ID: idMain + "$1", Name: "main$1", Package: app, Parent: idMain, Pos: "main.go:8:2"},
			{ID: "(" + app + ".run.getter#1).Get", Name: "Get", Package: app, Owner: app + ".run.getter#1", Pos: "main.go:31:20"},
		},
		Calls: []graph.FunctionCall{
			{Caller: idMain, Callee: idGet, Pos: "main.go:5:2", CallKind: graph.StaticCall},
			{Caller: idMain, Callee: idGet, Pos: "main.go:6:2", CallKind: graph.StaticCall},
			{Caller: idMain, Callee: "(" + tGetter + ").Get", Pos: "main.go:7:2", CallKind: graph.InterfaceCall},
			{Caller: idMain + "$1", Callee: store + ".Get", Pos: "main.go:9:3", CallKind: graph.StaticCall},
			{Caller: idMain, Callee: "os.Exit", Pos: "main.go:12:2", CallKind: graph.StaticCall},
		},
		Accesses: []graph.Access{
			{Accessor: store + ".New", Target: fUsers, Pos: "store/store.go:20:2", Kind: graph.ConstructAccess},
			{Accessor: idGet, Target: fUsers, Pos: "store/store.go:21:2", Kind: graph.AssignAccess},
			{Accessor: idGet, Target: fUsers, Pos: "store/store.go:22:2", Kind: graph.UpdateAccess},
			{Accessor: idGet, Target: fUsers, Pos: "store/store.go:23:2", Kind: graph.ContentAccess},
			{Accessor: idGet, Target: fUsers, Pos: "store/store.go:24:2", Kind: graph.ReadAccess},
			{Accessor: idGet, Target: fUsers, Pos: "store/store.go:25:2", Kind: graph.EscapeAccess},
			{Accessor: idGet, Target: tStore + ".hits", Pos: "store/store.go:27:2", Kind: graph.UpdateAccess},
			{Accessor: store, Target: tStore + ".hits", Pos: "store/store.go:8:2", Kind: graph.UpdateAccess}, // package level
			{Accessor: idGet, Target: tStore + ".hits", Pos: "store/store.go:28:2", Kind: graph.EscapeAccess},
			{Accessor: idGet, Target: tStore + ".label", Pos: "store/store.go:29:2", Kind: graph.UpdateAccess},
			{Accessor: store + ".New", Target: tStore + ".created", Pos: "store/store.go:20:9", Kind: graph.ConstructAccess},
			{Accessor: idGet, Target: store + ".maxGets", Pos: "store/store.go:26:2", Kind: graph.ReadAccess},
		},
		TypeRefs: []graph.TypeRef{
			{Referrer: idMain, Type: tStore, Pos: "main.go:4:2"},
			{Referrer: idMain, Type: tStore, Pos: "main.go:4:9"},
			{Referrer: idMain, Type: tStore, Pos: "main.go:8:2"},
			{Referrer: idMain, Type: tGetter, Pos: "main.go:7:9"},
			{Referrer: store, Type: tGetter, Pos: "store/cache.go:9:9"},
			{Referrer: idMain, Type: tCache, Pos: "main.go:10:2"},
			{Referrer: idMain, Type: "io.Writer", Pos: "main.go:11:2"}, // outside the repo
		},
		Implementations: []graph.Implementation{
			{Type: tStore, Interface: tGetter, PointerOnly: true},
			{Type: tCache, Interface: tGetter},
		},
		External: []graph.Function{{ID: "os.Exit", Name: "Exit", Package: "os"}},
	}
	return New(pg, cg)
}

// format is a match as the tests write it: kind, short name and counts.
func (ix *Index) format(m Match) string {
	var counts string
	switch m.Kind {
	case FuncDecl, MethodDecl:
		counts = fmt.Sprintf("callers %d via %d", m.Callers, m.Via)
	case FieldDecl, VarDecl, ConstDecl:
		counts = fmt.Sprintf("writes %d reads %d", m.Writes, m.Reads)
	default:
		counts = fmt.Sprintf("refs %d methods %d impls %d", m.Refs, m.Methods, m.Implementations)
	}
	return fmt.Sprintf("%s %s %s: %s, uses %d", m.Pos, m.Kind, ix.Short(m.ID), counts, m.Uses)
}

func TestFind(t *testing.T) {
	ix := findIndex()
	for _, tc := range []struct {
		word  string
		kinds []DeclKind
		want  []string
	}{
		// Most used first; ties by position, the line and column as numbers.
		// Interface calls count only in via; the closure and the local
		// type's method aren't listed.
		{word: "get", want: []string{
			"store/store.go:5:6 interface store.Getter: refs 2 methods 1 impls 2, uses 2",
			"store/store.go:14:17 method (*store.Store).Get: callers 2 via 1, uses 2",
			"store/get.go:3:6 func store.Get: callers 1 via 0, uses 1",
			"store/store.go:2:7 const store.maxGets: writes 0 reads 1, uses 1",
			"store/store.go:6:2 method (store.Getter).Get: callers 1 via 0, uses 1",
			"store/cache.go:5:16 method (store.Cache).Get: callers 0 via 1, uses 0",
			"store/get.go:10:6 func store.getAll: callers 0 via 0, uses 0",
			"store/store.go:11:2 field store.Store.getCount: writes 0 reads 0, uses 0",
		}},
		{word: "GET", kinds: []DeclKind{MethodDecl}, want: []string{
			"store/store.go:14:17 method (*store.Store).Get: callers 2 via 1, uses 2",
			"store/store.go:6:2 method (store.Getter).Get: callers 1 via 0, uses 1",
			"store/cache.go:5:16 method (store.Cache).Get: callers 0 via 1, uses 0",
		}},
		// Writes are construct, assign, update and content; reads are read and escape.
		{word: "users", want: []string{"store/store.go:10:2 field store.Store.users: writes 4 reads 2, uses 6"}},
		{word: "max", want: []string{"store/store.go:2:7 const store.maxGets: writes 0 reads 1, uses 1"}},
		// A plain word matches a declaration's own name only: not (store.Cache).Get.
		{word: "cache", want: []string{
			"store/cache.go:3:6 struct store.Cache: refs 1 methods 1 impls 0, uses 1",
			"store/cache.go:8:5 var store.cache: writes 0 reads 0, uses 0",
		}},
		{word: "Store", want: []string{"store/store.go:9:6 struct store.Store: refs 3 methods 1 impls 0, uses 3"}},
		{word: "i", kinds: []DeclKind{AliasDecl, TypeDecl}, want: []string{
			"store/store.go:3:6 alias store.ID: refs 0 methods 0 impls 0, uses 0",
		}},
		{word: "count", kinds: []DeclKind{TypeDecl}, want: []string{
			"store/store.go:4:6 type store.Count: refs 0 methods 0 impls 0, uses 0",
		}},
		// A word with a dot matches the short name, ( ) and * removed.
		{word: "Cache.", want: []string{
			"store/cache.go:3:20 field store.Cache.hits: writes 0 reads 0, uses 0",
			"store/cache.go:5:16 method (store.Cache).Get: callers 0 via 1, uses 0",
		}},
		// A substring still: store.Get is in store.Getter.Get.
		{word: "(*Store).Get", kinds: []DeclKind{MethodDecl}, want: []string{
			"store/store.go:14:17 method (*store.Store).Get: callers 2 via 1, uses 2",
			"store/store.go:6:2 method (store.Getter).Get: callers 1 via 0, uses 1",
		}},
		{word: "main.", want: []string{"main.go:3:6 func main.main: callers 0 via 0, uses 0"}},
		{word: "nope", want: nil},
	} {
		var got []string
		for _, m := range ix.Find(tc.word, tc.kinds) {
			got = append(got, ix.format(m))
		}
		if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
			t.Errorf("Find(%q, %v):\n%s\nwant:\n%s", tc.word, tc.kinds, strings.Join(got, "\n"), strings.Join(tc.want, "\n"))
		}
	}
}

// Callers plus via is what Callers returns: what `callers X | wc -l` counts.
func TestFindMatchesCallers(t *testing.T) {
	ix := findIndex()
	for _, m := range ix.Find("", []DeclKind{FuncDecl, MethodDecl}) {
		if n := len(ix.Callers(ix.Function(m.ID))); m.Callers+m.Via != n {
			t.Errorf("%s: callers %d + via %d, want %d sites", m.ID, m.Callers, m.Via, n)
		}
	}
}
