package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestState(t *testing.T) {
	t.Parallel()
	at := func(file string) string { return filepath.Join(fixture, filepath.FromSlash(file)) }
	for _, tc := range []struct {
		args []string
		want []string // stdout lines
		code int
	}{
		// A struct: a line per field, a role where the writes settle one, escapes apart.
		{[]string{"Store"}, []string{
			at("store/store.go:17:2") + ": store.Store.mu: mu    sync.Mutex  [no writes found, 2 escapes]",
			at("store/store.go:18:2") + ": store.Store.users: users map[int]model.User  [set at construction: 1 construct]",
		}, 0},
		{[]string{"store.Holder"}, []string{
			at("store/reads.go:22:2") + ": store.Holder.Base: Base  [no writes found, 1 escape]",
			at("store/reads.go:23:2") + ": store.Holder.N: N     int  [counter: 2 updates, 1 escape]",
			at("store/reads.go:24:2") + ": store.Holder.Step: Step  int  [no writes found]",
			at("store/reads.go:25:2") + ": store.Holder.Key: Key   int  [no writes found]",
			at("store/reads.go:26:2") + ": store.Holder.Items: Items []int  [1 assign, 1 construct, 1 update, 4 escapes]",
			at("store/reads.go:27:2") + ": store.Holder.Users: Users map[int]string  [1 content write, 2 escapes]",
			at("store/reads.go:28:2") + ": store.Holder.Val: Val   Part  [1 content write, 6 escapes]",
			at("store/reads.go:29:2") + ": store.Holder.Ptr: Ptr   *Part  [1 content write]",
			at("store/reads.go:30:2") + ": store.Holder.Sub: Sub   Sub  [no writes found]",
			at("store/reads.go:31:2") + ": store.Holder.H: H     http.Handler  [no writes found]",
			at("store/reads.go:32:2") + ": store.Holder.OnErr: OnErr func(error)  [no writes found]",
			at("store/reads.go:33:2") + ": store.Holder.mu: mu    sync.Mutex  [no writes found, 1 escape]",
		}, 0},
		// An alias of a struct is the struct.
		{[]string{"S"}, []string{
			at("store/store.go:17:2") + ": store.Store.mu: mu    sync.Mutex  [no writes found, 2 escapes]",
			at("store/store.go:18:2") + ": store.Store.users: users map[int]model.User  [set at construction: 1 construct]",
		}, 0},
		// A field: writes and escapes by position, reads with --reads.
		{[]string{"Holder.Items"}, []string{
			at("store/reads.go:87:2") + ": (*store.Holder).writes: r.Items = append(r.Items, 1)  [update]",
			at("store/reads.go:128:10") + ": (*store.Holder).hands: use(sum(r.Items))  [escape]",
			at("store/reads.go:130:2") + ": (*store.Holder).hands: other.Items = r.Items  [assign]",
			at("store/reads.go:130:16") + ": (*store.Holder).hands: other.Items = r.Items  [escape]",
			at("store/reads.go:131:13") + ": (*store.Holder).hands: _ = Holder{Items: r.Items}  [construct]",
			at("store/reads.go:131:20") + ": (*store.Holder).hands: _ = Holder{Items: r.Items}  [escape]",
			at("store/reads.go:132:8") + ": (*store.Holder).hands: ch <- r.Items  [escape]",
		}, 0},
		{[]string{"--reads", "Store.users"}, []string{
			at("store/store.go:21:35") + ": store.New: func New() *Store { return &Store{users: map[int]model.User{}} }  [construct]",
			at("store/store.go:26:9") + ": (*store.Store).Get: return s.users[id]  [read]",
		}, 0},
		// A package var, written at package level.
		{[]string{"defaultStore"}, []string{
			at("store/vars.go:4:5") + ": store (package level): var defaultStore = New()  [construct]",
		}, 0},
		// Nothing writes a const; nothing found.
		{[]string{"maxUsers"}, nil, 1},
		{[]string{"Getter"}, nil, 2},           // an interface
		{[]string{"Users"}, nil, 2},            // ambiguous
		{[]string{"Nope"}, nil, 2},             // not found
		{[]string{"--reads", "Store"}, nil, 2}, // --reads on a struct
		{[]string{"--format", "mermaid", "Store"}, nil, 2},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			t.Parallel()
			stdout, stderr, code := run(t, append([]string{"state"}, append(tc.args, fixture)...)...)
			got := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
			if stdout == "" {
				got = nil
			}
			if code != tc.code || strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
				t.Errorf("state %v: exit %d, stdout:\n%s\nwant exit %d:\n%s\nstderr: %s",
					tc.args, code, stdout, tc.code, strings.Join(tc.want, "\n"), stderr)
			}
		})
	}
}

// An ambiguous name lists the declarations on stderr, with the find hint.
func TestStateAmbiguous(t *testing.T) {
	t.Parallel()
	_, stderr, code := run(t, "state", "Users", fixture)
	want := "Users matches 2 declarations; name one of them:\n" +
		filepath.Join(fixture, "store", "writes.go") + ":21:2: store.Box.Users\n" +
		filepath.Join(fixture, "store", "reads.go") + ":27:2: store.Holder.Users\n" +
		"grepo find Users ranks them by use\n"
	if code != 2 || stderr != want {
		t.Errorf("state Users: exit %d, stderr:\n%s\nwant exit 2, stderr:\n%s", code, stderr, want)
	}
}

func TestStateJSON(t *testing.T) {
	t.Parallel()
	stdout, stderr, code := run(t, "state", "--json", "Holder", fixture)
	var ts typeStateJSON
	if err := json.Unmarshal([]byte(stdout), &ts); err != nil || code != 0 {
		t.Fatalf("state --json Holder: exit %d, %v\nstderr: %s", code, err, stderr)
	}
	if ts.Type.ID != "example.com/simple/store.Holder" || ts.Type.Fields != nil || len(ts.Fields) != 12 {
		t.Fatalf("type %s, its fields %v, %d fields; want Holder, none, 12", ts.Type.ID, ts.Type.Fields, len(ts.Fields))
	}
	n := ts.Fields[1]
	if n.ID != "example.com/simple/store.Holder.N" || n.Role != "counter" || n.Writes["update"] != 2 || len(n.Writes) != 1 ||
		n.Escapes != 1 || n.Line != "N     int" || n.Pos != filepath.Join(fixture, "store", "reads.go")+":23:2" {
		t.Errorf("Holder.N = %+v", n)
	}
	if step := ts.Fields[2]; step.Role != "no writes found" || step.Writes == nil || len(step.Writes) != 0 {
		t.Errorf("Holder.Step = %+v, want no writes, writes {}", step)
	}

	stdout, stderr, code = run(t, "state", "--json", "--reads", "Store.users", fixture)
	var as accessesJSON
	if err := json.Unmarshal([]byte(stdout), &as); err != nil || code != 0 {
		t.Fatalf("state --json Store.users: exit %d, %v\nstderr: %s", code, err, stderr)
	}
	if as.Field == nil || as.Field.ID != "example.com/simple/store.Store.users" || as.Var != nil || len(as.Sites) != 2 {
		t.Fatalf("field %+v, var %+v, %d sites", as.Field, as.Var, len(as.Sites))
	}
	if s := as.Sites[0]; s.Kind != "write" || s.Access != "construct" || s.From != "example.com/simple/store.New" {
		t.Errorf("site 0 = %+v", s)
	}
	if s := as.Sites[1]; s.Kind != "read" || s.Access != "read" {
		t.Errorf("site 1 = %+v", s)
	}

	stdout, _, code = run(t, "state", "--json", "maxUsers", fixture)
	as = accessesJSON{}
	if err := json.Unmarshal([]byte(stdout), &as); err != nil || code != 1 || as.Field != nil || as.Var == nil || !as.Var.Const || as.Sites == nil || len(as.Sites) != 0 {
		t.Errorf("state --json maxUsers: exit %d, var %+v, sites %v; want exit 1, the const, []", code, as.Var, as.Sites)
	}
}
