package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestFind(t *testing.T) {
	t.Parallel()
	at := func(file string) string { return filepath.Join(fixture, filepath.FromSlash(file)) }
	for _, tc := range []struct {
		args []string
		want []string // stdout lines
		code int
	}{
		// Ranked by direct callers; via an interface only in the tag.
		{[]string{"get"}, []string{
			at("store/store.go:12:6") + ": store.Getter: type Getter interface {  [interface, 2 refs, 3 implementations]",
			at("store/store.go:13:2") + ": (store.Getter).Get: Get(id int) model.User  [method, 1 caller]",
			at("store/store.go:23:17") + ": (*store.Store).Get: func (s *Store) Get(id int) model.User {  [method, 1 caller, 1 more via an interface]",
			at("store/cache.go:9:16") + ": (store.Cache).Get: func (c Cache) Get(id int) model.User { return model.User{ID: id} }  [method, no callers, 1 via an interface]",
		}, 0},
		{[]string{"--kind", "field,var", "--kind", "const", "users"}, []string{
			at("store/vars.go:21:7") + ": store.maxUsers: const maxUsers = 100  [const, 10 reads]",
			at("store/reads.go:27:2") + ": store.Holder.Users: Users map[int]string  [field, 1 write, 4 reads]",
			at("store/writes.go:21:2") + ": store.Box.Users: Users map[int]model.User  [field, 3 writes, no reads]",
			at("store/store.go:18:2") + ": store.Store.users: users map[int]model.User  [field, 1 write, 1 read]",
		}, 0},
		{[]string{"--kind", "struct", "Store"}, []string{
			at("store/store.go:16:6") + ": store.Store: type Store struct {  [struct, 5 refs, 2 methods]",
		}, 0},
		// A dot: the qualified name, so List's methods, not other Pushes.
		{[]string{"List.P"}, []string{
			at("main.go:56:19") + ": (*main.List[T]).Push: func (l *List[T]) Push(v T) { l.items = append(l.items, v) }  [method, 1 caller]",
		}, 0},
		{[]string{"nope"}, nil, 1},
		{[]string{"--kind", "class", "get"}, nil, 2},
		{[]string{"--format", "dot", "get"}, nil, 2},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			t.Parallel()
			stdout, stderr, code := run(t, append([]string{"find"}, append(tc.args, fixture)...)...)
			got := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
			if stdout == "" {
				got = nil
			}
			if code != tc.code || strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
				t.Errorf("find %v: exit %d, stdout:\n%s\nwant exit %d:\n%s\nstderr: %s",
					tc.args, code, stdout, tc.code, strings.Join(tc.want, "\n"), stderr)
			}
		})
	}
}

// --json has each result's kind's counts, and [] when nothing matches.
func TestFindJSON(t *testing.T) {
	t.Parallel()
	stdout, stderr, code := run(t, "find", "--json", "--kind", "method,field,interface", "Store.", fixture)
	var out struct {
		Base, Word string
		Results    []map[string]any
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil || code != 0 {
		t.Fatalf("find --json: exit %d, %v\nstdout: %s\nstderr: %s", code, err, stdout, stderr)
	}
	if out.Word != "Store." || out.Base == "" {
		t.Errorf("word %q, base %q", out.Word, out.Base)
	}
	got := map[string]map[string]any{}
	for _, r := range out.Results {
		got[r["id"].(string)] = r
	}
	for id, want := range map[string]map[string]any{
		"(*example.com/simple/store.Store).Get": {"kind": "method", "uses": 1.0, "callers": 1.0, "via": 1.0,
			"pos": filepath.Join(fixture, "store", "store.go") + ":23:17", "line": "func (s *Store) Get(id int) model.User {"},
		"example.com/simple/store.Store.users": {"kind": "field", "uses": 2.0, "writes": 1.0, "reads": 1.0},
		"example.com/simple/store.Store.mu":    {"kind": "field", "uses": 2.0, "writes": 0.0, "reads": 2.0}, // s.mu.Lock(): escapes
	} {
		r := got[id]
		for k, v := range want {
			if r[k] != v {
				t.Errorf("%s: %s = %v, want %v", id, k, r[k], v)
			}
		}
		for _, k := range []string{"refs", "methods", "implementations"} {
			if _, ok := r[k]; ok {
				t.Errorf("%s has %s", id, k)
			}
		}
	}

	stdout, _, code = run(t, "find", "--json", "nope", fixture)
	if code != 1 || !strings.Contains(stdout, `"results": []`) {
		t.Errorf("find --json nope: exit %d, stdout:\n%s\nwant exit 1 and results []", code, stdout)
	}
}
