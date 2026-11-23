package cli

import (
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestCallees(t *testing.T) {
	t.Parallel()
	at := func(file string) string { return filepath.Join(fixture, filepath.FromSlash(file)) }
	for _, tc := range []struct {
		args []string
		want []string // stdout lines
		code int
	}{
		// One level: a line per site, by position.
		{[]string{"main"}, []string{
			at("main.go:16:7") + ": store.New: s := store.New()",
			at("main.go:17:7") + ": (*store.Store).Get: u := s.Get(1)",
			at("main.go:20:2") + ": (store.Getter).Get: g.Get(2)  [interface]",
			at("main.go:23:2") + ": main.greet: f(u.Name)  [through a local]",
			at("main.go:25:2") + ": main.main$1: func() {",
			at("main.go:32:2") + ": fmt.Println: fmt.Println(u)",
		}, 0},
		// Two levels: implementations, see above, outside leaves.
		{[]string{"--depth", "2", "main"}, []string{
			at("main.go:16:7") + ": store.New: s := store.New()",
			at("main.go:17:7") + ": (*store.Store).Get: u := s.Get(1)",
			at("store/store.go:24:2") + ":   (*sync.Mutex).Lock: s.mu.Lock()",
			at("store/store.go:25:8") + ":   (*sync.Mutex).Unlock: defer s.mu.Unlock()  [defer]",
			at("main.go:20:2") + ": (store.Getter).Get: g.Get(2)  [interface]",
			at("store/cache.go:9:16") + ":   (store.Cache).Get  [implements store.Getter]",
			at("store/store.go:23:17") + ":   (*store.Store).Get  [implements store.Getter, see above]",
			at("main.go:23:2") + ": main.greet: f(u.Name)  [through a local]",
			at("main.go:36:2") + `:   fmt.Println: fmt.Println("hello", name)`,
			at("main.go:25:2") + ": main.main$1: func() {",
			at("main.go:26:3") + `:   main.greet: greet("closure")  [see above]`,
			at("main.go:32:2") + ": fmt.Println: fmt.Println(u)",
		}, 0},
		{[]string{"--direct", "main"}, []string{
			at("main.go:16:7") + ": store.New: s := store.New()",
			at("main.go:17:7") + ": (*store.Store).Get: u := s.Get(1)",
			at("main.go:25:2") + ": main.main$1: func() {",
			at("main.go:32:2") + ": fmt.Println: fmt.Println(u)",
		}, 0},
		// Values, go and defer, a call through a field.
		{[]string{"Runner.closures"}, []string{
			at("store/closures.go:30:2") + ": (*store.Runner).closures$1: func() {",
			at("store/closures.go:34:2") + ": (*store.Runner).closures$2: inc()  [through a local]",
			at("store/closures.go:35:5") + ": (*store.Runner).start: go r.start()  [go]",
			at("store/closures.go:36:8") + ": (*store.Runner).start: defer r.start()  [defer]",
			at("store/closures.go:37:5") + ": (*store.Runner).closures$3: go func() { r.start() }()  [go]",
			at("store/closures.go:38:8") + ": (*store.Runner).closures$4: defer func() {}()  [defer]",
			at("store/closures.go:39:2") + ": sort.Slice: sort.Slice(xs, func(i, j int) bool { return xs[i] < xs[j] })",
			at("store/closures.go:39:2") + ": (*store.Runner).closures$5: sort.Slice(xs, func(i, j int) bool { return xs[i] < xs[j] })  [passed to sort.Slice]",
			at("store/closures.go:40:2") + `: http.HandleFunc: http.HandleFunc("/a", handle)`,
			at("store/closures.go:40:2") + `: store.handle: http.HandleFunc("/a", handle)  [passed to http.HandleFunc]`,
			at("store/closures.go:41:2") + `: http.HandleFunc: http.HandleFunc("/b", r.serve)`,
			at("store/closures.go:41:2") + `: (*store.Runner).serve: http.HandleFunc("/b", r.serve)  [passed to http.HandleFunc]`,
			at("store/closures.go:42:2") + ": store.mapAll: mapAll(xs, double[string])",
			at("store/closures.go:42:2") + ": store.double: mapAll(xs, double[string])  [passed to store.mapAll]",
			at("store/closures.go:43:2") + ": (*store.Runner).closures$6: r.OnDone = func() {}  [stored in store.Runner.OnDone]",
			at("store/closures.go:44:2") + ": (*store.Runner).start: r.OnDone = r.start  [stored in store.Runner.OnDone]",
			at("store/closures.go:45:13") + ": (*store.Runner).closures$7: _ = Runner{Run: func() error { return nil }}  [stored in store.Runner.Run]",
			at("store/closures.go:46:2") + ": store.Runner.OnDone: r.OnDone()  [dynamic]",
		}, 0},
		// A call nothing resolves.
		{[]string{"--depth", "0", "Runner.locals"}, []string{
			at("store/closures.go:63:2") + ": (*store.Runner).locals$1: f()  [through a local]",
			at("store/closures.go:63:2") + ": (*store.Runner).start: f()  [through a local]",
			at("store/closures.go:64:2") + ": (*store.Runner).locals$2: func() { f() }()",
			at("store/closures.go:64:11") + ":   (*store.Runner).locals$1: func() { f() }()  [through a local]",
			at("store/closures.go:64:11") + ":   (*store.Runner).start: func() { f() }()  [through a local]",
			at("store/closures.go:68:4") + ": store.handleLater: g()  [through a local]",
			at("store/closures.go:72:2") + ": (func value): given()  [dynamic]",
		}, 0},
		// Calls nothing: 1, as grep's.
		{[]string{"Fake.seal"}, nil, 1},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			t.Parallel()
			stdout, stderr, code := run(t, append([]string{"callees"}, append(tc.args, fixture)...)...)
			got := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
			if stdout == "" {
				got = nil
			}
			if code != tc.code || strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
				t.Errorf("callees %v: exit %d, stdout:\n%s\nwant exit %d:\n%s\nstderr: %s",
					tc.args, code, stdout, tc.code, strings.Join(tc.want, "\n"), stderr)
			}
		})
	}
}

// --json nests children and marks seen sites.
func TestCalleesJSON(t *testing.T) {
	t.Parallel()
	stdout, stderr, code := run(t, "callees", "--json", "--depth", "2", "main", fixture)
	var out calleesJSON
	if err := json.Unmarshal([]byte(stdout), &out); err != nil || code != 0 {
		t.Fatalf("callees --json: exit %d, %v\nstderr: %s", code, err, stderr)
	}
	if out.Function.ID != "example.com/simple.main" || len(out.Callees) != 6 || out.Base == "" {
		t.Fatalf("function %s, %d callees, base %q; want main, 6, a base", out.Function.ID, len(out.Callees), out.Base)
	}
	iface := out.Callees[2]
	if iface.Kind != "interface" || iface.Via != "example.com/simple/store.Getter" || len(iface.Children) != 2 {
		t.Fatalf("interface call = %+v, want via Getter with 2 implementations", iface)
	}
	impl := iface.Children[1]
	if impl.Kind != "implementation" || impl.Target != "(*example.com/simple/store.Store).Get" || !impl.Seen ||
		impl.Line != "" || impl.Pos != filepath.Join(fixture, "store", "store.go")+":23:17" {
		t.Errorf("implementation = %+v, want (*Store).Get at its declaration, seen, no line", impl)
	}
	if c := out.Callees[4]; c.Target != "example.com/simple.main$1" || c.FromDecl != "" || len(c.Children) != 1 ||
		c.Children[0].From != "example.com/simple.main$1" || c.Children[0].FromDecl != "example.com/simple.main" {
		t.Errorf("closure call = %+v, want main$1 with its call from inside it", c)
	}
}

func TestCalleesBadDepth(t *testing.T) {
	t.Parallel()
	root := newRootCmd(&app{})
	root.SetArgs([]string{"callees", "--depth", "-1", "main", fixture})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	if err := root.ExecuteContext(t.Context()); ExitCode(err) != 2 || !strings.Contains(err.Error(), "--depth -1") {
		t.Errorf("--depth -1: err %v, want exit 2 naming the depth", err)
	}
}
