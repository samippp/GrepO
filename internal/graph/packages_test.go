// packages_test.go checks BuildPackageGraph against testdata/simple, a tiny
// real module (main → store → model). The fixture is loaded through load.Load
// rather than mocked, so this also tests the loader end to end.

package graph

import (
	"slices"
	"testing"

	"github.com/samippp/grepo/internal/load"
)

func TestBuildPackageGraph(t *testing.T) {
	g := BuildPackageGraph(loadFixture(t))

	if g.Module != "example.com/simple" {
		t.Errorf("module = %q, want example.com/simple", g.Module)
	}
	if len(g.Errors) > 0 {
		t.Errorf("unexpected load errors: %v", g.Errors)
	}

	wantEdges := []Edge{
		{From: "example.com/simple", To: "example.com/simple/store"},
		{From: "example.com/simple/store", To: "example.com/simple/model"},
	}
	if !slices.Equal(g.Edges, wantEdges) {
		t.Errorf("edges = %v, want %v", g.Edges, wantEdges)
	}

	var store *Package
	for i := range g.Packages {
		if g.Packages[i].Path == "example.com/simple/store" {
			store = &g.Packages[i]
		}
	}
	if store == nil {
		t.Fatal("store package missing")
	}
	if store.Dir != "store" {
		t.Errorf("store dir = %q, want store", store.Dir)
	}
	if !slices.Equal(store.Imports.Stdlib, []string{"io", "net/http", "os", "sort", "sync"}) {
		t.Errorf("store stdlib imports = %v, want [io net/http os sort sync]", store.Imports.Stdlib)
	}
}

// Imports are split by what go list says, not by their paths: a dependency's
// module path needn't have a dot (replace tools => ./tools), and the
// standard library's never does.
func TestBuildPackageGraphStandardFromLoad(t *testing.T) {
	res := &load.Result{
		Dir: "/repo",
		Packages: []*load.Package{
			{PkgPath: "app", Name: "main", Dir: "/repo", Imports: []string{"app/util", "fmt", "tools"}},
			{PkgPath: "app/util", Name: "util", Dir: "/repo/util"},
		},
		Repo:     map[string]bool{"app": true, "app/util": true},
		Standard: map[string]bool{"fmt": true},
	}
	got := BuildPackageGraph(res).Packages[0].Imports
	want := Imports{Internal: []string{"app/util"}, External: []string{"tools"}, Stdlib: []string{"fmt"}}
	if !slices.Equal(got.Internal, want.Internal) || !slices.Equal(got.External, want.External) || !slices.Equal(got.Stdlib, want.Stdlib) {
		t.Errorf("imports = %+v, want %+v", got, want)
	}
}
