package cli

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"
)

// A directory inside a module means the whole module, for every command:
// analyze's graph from store/ is the module's, positioned from its root, as
// callers' sites from there are.
func TestAnalyzeSubdirectory(t *testing.T) {
	stdout, stderr, code := run(t, "analyze", "--no-cache", filepath.Join(fixture, "store"))
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var g struct {
		Root     string
		Packages []struct{ Path, Dir string }
	}
	if err := json.Unmarshal([]byte(stdout), &g); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if g.Root != filepath.ToSlash(root) {
		t.Errorf("root = %s, want the module's, %s", g.Root, filepath.ToSlash(root))
	}
	var dirs []string
	for _, p := range g.Packages {
		dirs = append(dirs, p.Dir)
	}
	if want := []string{".", "model", "store"}; !slices.Equal(dirs, want) {
		t.Errorf("package dirs = %v, want %v", dirs, want)
	}
}

// callers from store/ finds the sites in the module root's package, main,
// both direct and through store.Getter, as it does from the root.
func TestCallersSubdirectory(t *testing.T) {
	stdout, stderr, code := run(t, "callers", "Store.Get", filepath.Join(fixture, "store"))
	want := filepath.Join(fixture, "main.go") + ":17: main.main: u := s.Get(1)\n" +
		filepath.Join(fixture, "main.go") + ":20: main.main: g.Get(2)  [via store.Getter]\n"
	if code != 0 || stdout != want {
		t.Errorf("callers Store.Get from store: exit %d, stdout:\n%s\nwant exit 0:\n%s\nstderr: %s", code, stdout, want, stderr)
	}
}
