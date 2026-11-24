package graph

import (
	"fmt"
	"testing"
)

func TestCallGraphVariables(t *testing.T) {
	g := buildFixtureCallGraph(t)

	v := func(name, typ, pos string) Variable {
		return Variable{ID: storePkg + "." + name, Name: name, Package: storePkg, Type: typ, Pos: "store/vars.go:" + pos}
	}
	c := func(name, typ, value, pos string, exported bool) Variable {
		x := v(name, typ, pos)
		x.Const, x.Value, x.Exported = true, value, exported
		return x
	}
	timeout := v("Timeout", "int", "6:5")
	timeout.Exported = true

	want := []Variable{
		v("defaultStore", "*"+tStore, "4:5"),
		timeout,
		// var a, b = 1, 2: one node per name.
		v("a", "int", "8:5"),
		v("b", "int", "8:8"),
		// var first, err = load(): one node per name, one call.
		v("first", "int", "11:5"),
		v("err", "error", "11:12"),
		// No nodes for the two "_" vars on lines 16-17.
		c("maxUsers", "untyped int", "100", "21:7", false),
		c("greeting", "string", `"hi"`, "23:7", false),
		// iota: Green and Blue have no value in the source, but the type checker knows it.
		c("Red", "untyped int", "0", "26:2", true),
		c("Green", "untyped int", "1", "27:2", true),
		c("Blue", "untyped int", "2", "28:2", true),
		// No node for save's local var count.
		{ID: storePkg + ".Counter", Name: "Counter", Package: storePkg, Type: "int", Exported: true, Pos: wat("25:5")},
		{ID: storePkg + ".defaultBox", Name: "defaultBox", Package: storePkg, Type: tWBox, Pos: wat("28:5")},
		{ID: storePkg + ".wrapped", Name: "wrapped", Package: storePkg, Type: storePkg + ".Wrapped", Pos: wat("91:5")},
		{ID: storePkg + ".limit", Name: "limit", Package: storePkg, Type: "int", Pos: rat("46:5")},
		{ID: storePkg + ".doubled", Name: "doubled", Package: storePkg, Type: "untyped int", Const: true, Value: "200", Pos: rat("48:7")},
		{ID: storePkg + ".shared", Name: "shared", Package: storePkg, Type: tRHolder, Pos: rat("50:5")},
		{ID: storePkg + ".OnReady", Name: "OnReady", Package: storePkg, Type: "func()", Exported: true, Pos: rat("52:5")},
		{ID: storePkg + ".table", Name: "table", Package: storePkg, Type: "[100]int", Pos: rat("164:5")},
		{ID: storePkg + ".hooks", Name: "hooks", Package: storePkg, Type: "func()", Pos: cat("49:5")},
		{ID: storePkg + ".fallback", Name: "fallback", Package: storePkg, Type: "func()", Pos: cat("49:12")},
		{ID: storePkg + ".nested", Name: "nested", Package: storePkg, Type: "func()", Pos: cat("53:5")},
		{ID: storePkg + ".timeout", Name: "timeout", Package: storePkg, Type: "time.Duration", Pos: "store/consts.go:16:5"},
		{ID: storePkg + ".defaultRef", Name: "defaultRef", Package: storePkg, Type: storePkg + ".Ref", Pos: "store/typerefs.go:30:5"},
		{ID: modelPkg + ".MaxTag", Name: "MaxTag", Package: modelPkg, Type: "untyped int", Const: true, Value: "8", Exported: true, Pos: "model/kinds.go:36:7"},
	}

	checkExact(t, "variable", g.Variables, want, func(v Variable) string { return fmt.Sprintf("%+v", v) })
}
