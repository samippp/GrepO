package query

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/samippp/grepo/internal/graph"
)

func TestTarget(t *testing.T) {
	ix := findIndex()
	for _, tc := range []struct {
		name, prefer string
		kind         DeclKind
		id           string
	}{
		{name: "Store", kind: StructDecl, id: tStore},
		{name: "store.Store", kind: StructDecl, id: tStore},
		{name: tStore, kind: StructDecl, id: tStore}, // an ID
		{name: "S", kind: StructDecl, id: tStore},    // an alias of a repo struct: the struct
		{name: "ID", kind: AliasDecl, id: store + ".ID"},
		{name: "Getter", kind: InterfaceDecl, id: tGetter},
		{name: "users", kind: FieldDecl, id: fUsers},
		{name: "Store.users", kind: FieldDecl, id: fUsers},
		{name: "store.Store.users", kind: FieldDecl, id: fUsers},
		{name: "app/store.Store.users", kind: FieldDecl, id: fUsers},
		{name: "Cache.hits", kind: FieldDecl, id: tCache + ".hits"},
		{name: "cache", kind: VarDecl, id: store + ".cache"}, // names are exact: not Cache
		{name: "store.cache", kind: VarDecl, id: store + ".cache"},
		{name: "maxGets", kind: ConstDecl, id: store + ".maxGets"},
		{name: app + ".run.getter#1.get", kind: FieldDecl, id: app + ".run.getter#1.get"}, // a local type's field: by ID
	} {
		got, err := ix.Target(tc.name, tc.prefer)
		if err != nil || got.Kind != tc.kind || got.ID != tc.id {
			t.Errorf("Target(%q) = %s %s, %v; want %s %s", tc.name, got.Kind, got.ID, err, tc.kind, tc.id)
		}
	}

	_, err := ix.Target("hits", "")
	var amb *AmbiguousError
	if !errors.As(err, &amb) {
		t.Fatalf("Target(hits) error = %v, want AmbiguousError", err)
	}
	var ids []string
	for _, c := range amb.Targets {
		ids = append(ids, c.ID)
	}
	if want := []string{tCache + ".hits", tStore + ".hits"}; !slices.Equal(ids, want) {
		t.Errorf("candidates = %v, want %v", ids, want)
	}
	var nf *NotFoundError
	for _, name := range []string{"nope", "get", "Cache.users", "cache.Store.users", "Get"} {
		if _, err := ix.Target(name, ""); !errors.As(err, &nf) {
			t.Errorf("Target(%q) error = %v, want NotFoundError", name, err)
		}
	}
}

func TestFields(t *testing.T) {
	ix := findIndex()
	st, _ := ix.Target("Store", "")
	var got []string
	for _, f := range ix.Fields(st.Type) {
		got = append(got, fmt.Sprintf("%s role=%q writes=%v escapes=%d", f.Field.Name, f.Role, f.Writes, f.Escapes))
	}
	// Declaration order. Reads aren't counted; escapes are, apart.
	want := []string{
		`users role="" writes=map[assign:1 construct:1 content:1 update:1] escapes=1`,
		`getCount role="no writes found" writes=map[] escapes=0`,
		`hits role="counter" writes=map[update:2] escapes=1`, // numeric, updates only
		`label role="" writes=map[update:1] escapes=0`,       // updates, but not a number
		`created role="set at construction" writes=map[construct:1] escapes=0`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("Fields(Store):\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestAccesses(t *testing.T) {
	ix := findIndex()
	format := func(s Site) string { return fmt.Sprintf("%s %s %s %s", s.Pos, s.Kind, s.Access.Kind, s.From) }
	for _, tc := range []struct {
		id    string
		reads bool
		want  []string
	}{
		// Writes and escapes by position; reads only when asked.
		{fUsers, false, []string{
			"store/store.go:20:2 write construct " + store + ".New",
			"store/store.go:21:2 write assign " + idGet,
			"store/store.go:22:2 write update " + idGet,
			"store/store.go:23:2 write content " + idGet,
			"store/store.go:25:2 escape escape " + idGet,
		}},
		{fUsers, true, []string{
			"store/store.go:20:2 write construct " + store + ".New",
			"store/store.go:21:2 write assign " + idGet,
			"store/store.go:22:2 write update " + idGet,
			"store/store.go:23:2 write content " + idGet,
			"store/store.go:24:2 read read " + idGet,
			"store/store.go:25:2 escape escape " + idGet,
		}},
		{tStore + ".hits", false, []string{
			"store/store.go:8:2 write update " + store, // package level
			"store/store.go:27:2 write update " + idGet,
			"store/store.go:28:2 escape escape " + idGet,
		}},
		{store + ".maxGets", false, nil},
		{store + ".maxGets", true, []string{"store/store.go:26:2 read read " + idGet}},
	} {
		var got []string
		for _, s := range ix.Accesses(tc.id, tc.reads) {
			got = append(got, format(s))
		}
		if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
			t.Errorf("Accesses(%s, %v):\n%s\nwant:\n%s", tc.id, tc.reads, strings.Join(got, "\n"), strings.Join(tc.want, "\n"))
		}
	}
}

// The counts Fields gives are the sites Accesses lists.
func TestFieldsMatchAccesses(t *testing.T) {
	ix := findIndex()
	st, _ := ix.Target("Store", "")
	for _, f := range ix.Fields(st.Type) {
		n := map[graph.AccessKind]int{}
		esc := 0
		for _, s := range ix.Accesses(f.Field.ID, false) {
			if s.Kind == Escape {
				esc++
			} else {
				n[s.Access.Kind]++
			}
		}
		if !maps.Equal(n, f.Writes) || esc != f.Escapes {
			t.Errorf("%s: Fields says %v, %d escapes; Accesses has %v, %d", f.Field.ID, f.Writes, f.Escapes, n, esc)
		}
	}
}
