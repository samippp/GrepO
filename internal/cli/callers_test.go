package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture is the graph package's test repo; printed paths are relative to
// this package's directory, where tests run.
var fixture = filepath.Join("..", "graph", "testdata", "simple")

// run runs grepo with args, as main does, and returns its stdout, stderr and
// exit code.
func run(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errOut bytes.Buffer
	root := newRootCmd(&app{})
	root.SetArgs(args)
	root.SetOut(&out)
	root.SetErr(&errOut)
	err := root.ExecuteContext(context.Background())
	return out.String(), errOut.String(), ExitCode(err)
}

func TestCallers(t *testing.T) {
	t.Parallel()
	at := func(file string) string { return filepath.Join(fixture, filepath.FromSlash(file)) }
	for _, tc := range []struct {
		args []string
		want []string // stdout lines
		code int
	}{
		// A direct call, then one through the interface *Store implements.
		{[]string{"Store.Get"}, []string{
			at("main.go:17:7") + ": main.main: u := s.Get(1)",
			at("main.go:20:2") + ": main.main: g.Get(2)  [via store.Getter]",
		}, 0},
		{[]string{"--direct", "Store.Get"}, []string{at("main.go:17:7") + ": main.main: u := s.Get(1)"}, 0},
		// Passed as a value.
		{[]string{"handle"}, []string{
			at("store/closures.go:40:2") + `: (*store.Runner).closures: http.HandleFunc("/a", handle)  [passed to http.HandleFunc]`,
		}, 0},
		// Through interfaces from outside the repo: the built-in error, and io.Writer.
		{[]string{"failure.Error"}, []string{
			at("main.go:78:42") + ": main.describe: func describe(err error) string { return err.Error() }  [via error]",
		}, 0},
		{[]string{"bufWriter.Write"}, []string{
			at("store/outside.go:12:27") + ": store.flush: func flush(w io.Writer) { w.Write(nil) }  [via io.Writer]",
		}, 0},
		// Promoted: conn has no Open, so a call through OpenCloser reaches its
		// Close only through the types embedding it.
		{[]string{"conn.Close"}, []string{
			at("store/promoted.go:35:32") + ": store.closeAll: func closeAll(oc OpenCloser) { oc.Close() }  [via store.OpenCloser]",
		}, 0},
		// Through a local: assigned after the call, in a loop.
		{[]string{"handleLater"}, []string{
			at("store/closures.go:68:4") + ": (*store.Runner).locals: g()  [through a local]",
		}, 0},
		{[]string{"--direct", "handleLater"}, nil, 1},
		// A function outside the repo.
		{[]string{"fmt.Println"}, []string{
			at("main.go:32:2") + ": main.main: fmt.Println(u)",
			at("main.go:36:2") + `: main.greet: fmt.Println("hello", name)`,
		}, 0},
		// Found, but nothing calls it: 1, as grep's.
		{[]string{"main.main"}, nil, 1},
		// Not found: an error.
		{[]string{"Nope"}, nil, 2},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			t.Parallel() // each run loads the fixture on its own
			stdout, stderr, code := run(t, append([]string{"callers"}, append(tc.args, fixture)...)...)
			got := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
			if stdout == "" {
				got = nil
			}
			if code != tc.code || strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
				t.Errorf("callers %v: exit %d, stdout:\n%s\nwant exit %d:\n%s\nstderr: %s",
					tc.args, code, stdout, tc.code, strings.Join(tc.want, "\n"), stderr)
			}
		})
	}
}

// An ambiguous name lists its candidates on stderr, each with a name that
// picks it, and exits with 2; the package in the directory given wins.
func TestCallersAmbiguous(t *testing.T) {
	t.Parallel()
	stdout, stderr, code := run(t, "callers", "seal", fixture)
	want := "seal matches 3 functions; name one of them:\n" +
		filepath.Join(fixture, "model", "impls.go") + ":8:24: (model.Sealed).seal\n" +
		filepath.Join(fixture, "model", "impls.go") + ":12:14: (model.Token).seal\n" +
		filepath.Join(fixture, "store", "impls.go") + ":8:13: (store.Fake).seal\n"
	if code != 2 || stdout != "" || stderr != want {
		t.Errorf("callers seal: exit %d, stdout %q, stderr:\n%s\nwant exit 2, stderr:\n%s", code, stdout, stderr, want)
	}

	// From the store directory, store's seal is the one meant. Nothing calls
	// it, so --json still names it, with no sites, and exits with 1.
	stdout, stderr, code = run(t, "callers", "--json", "seal", filepath.Join(fixture, "store"))
	var out callersJSON
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("callers --json seal: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if code != 1 || out.Function.ID != "(example.com/simple/store.Fake).seal" || out.Sites == nil || len(out.Sites) != 0 {
		t.Errorf("callers --json seal from store: exit %d, function %s, sites %v; want exit 1, (store.Fake).seal, []",
			code, out.Function.ID, out.Sites)
	}
}

func TestCallersJSON(t *testing.T) {
	t.Parallel()
	stdout, stderr, code := run(t, "callers", "--json", "Store.Get", fixture)
	var out callersJSON
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("callers --json: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if code != 0 || out.Function.ID != "(*example.com/simple/store.Store).Get" ||
		out.Function.Pos != filepath.Join(fixture, "store", "store.go")+":23:17" {
		t.Errorf("function = %+v, exit %d", out.Function, code)
	}
	if len(out.Sites) != 2 {
		t.Fatalf("sites = %+v, want 2", out.Sites)
	}
	direct, iface := out.Sites[0], out.Sites[1]
	if direct.Kind != "direct" || direct.Pos != filepath.Join(fixture, "main.go")+":17:7" ||
		direct.From != "example.com/simple.main" || direct.Line != "u := s.Get(1)" ||
		direct.CallKind != "static" || len(direct.Args) != 1 {
		t.Errorf("direct site = %+v", direct)
	}
	if iface.Kind != "interface" || iface.Via != "example.com/simple/store.Getter" ||
		iface.Callee != "(example.com/simple/store.Getter).Get" || iface.CallKind != "interface" {
		t.Errorf("interface site = %+v", iface)
	}
}

// --json says where positions are relative to, and which declaration a
// closure site is written in.
func TestCallersJSONBaseAndDecl(t *testing.T) {
	t.Parallel()
	stdout, stderr, code := run(t, "callers", "--format", "json", "greet", fixture)
	var out callersJSON
	if err := json.Unmarshal([]byte(stdout), &out); err != nil || code != 0 {
		t.Fatalf("callers --format json greet: exit %d, %v\nstderr: %s", code, err, stderr)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if out.Base != filepath.ToSlash(wd) {
		t.Errorf("base = %s, want the working directory, %s", out.Base, filepath.ToSlash(wd))
	}
	decls := map[string]string{}
	for _, s := range out.Sites {
		decls[s.From] = s.FromDecl
	}
	want := map[string]string{
		"example.com/simple.main$1":         "example.com/simple.main", // greet("closure") in func(){…}()
		"example.com/simple.main":           "",
		"example.com/simple.init@main.go#1": "",
		"example.com/simple.init@main.go#2": "",
	}
	if !maps.Equal(decls, want) {
		t.Errorf("fromDecl by from = %v, want %v", decls, want)
	}
}

// A format the command doesn't print, or --json with --format, is an
// error, before anything is loaded.
func TestCallersFormatErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		args []string
		want string // in the error
	}{
		{[]string{"--format", "mermaid"}, "--format mermaid: callers prints text or json"},
		{[]string{"--json", "--format", "text"}, "[format json] were all set"},
	} {
		root := newRootCmd(&app{})
		root.SetArgs(append(append([]string{"callers"}, tc.args...), "greet", fixture))
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		err := root.ExecuteContext(t.Context())
		if ExitCode(err) != 2 || err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("callers %v: err %v; want exit 2 and %q", tc.args, err, tc.want)
		}
	}
}
