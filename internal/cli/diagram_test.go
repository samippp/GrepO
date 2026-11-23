package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestDiagrams(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		args []string
		want string
		code int
	}{
		// Interface methods are nodes, with dotted edges to their
		// implementations; a repeat is an edge to the node already there.
		{[]string{"callees", "--depth", "2", "--format", "mermaid", "main"}, `flowchart LR
  n0["main.main"]
  n1["store.New"]
  n2["(*store.Store).Get"]
  n3["(*sync.Mutex).Lock"]
  n4["(*sync.Mutex).Unlock"]
  n5{{"(store.Getter).Get"}}
  n6["(store.Cache).Get"]
  n7["main.greet"]
  n8["fmt.Println"]
  n9["main.main$1"]
  n0 --> n1
  n0 --> n2
  n2 --> n3
  n2 --> n4
  n0 --> n5
  n5 -.-> n6
  n5 -.-> n2
  n0 -- "through a local" --> n7
  n7 --> n8
  n0 --> n9
  n9 --> n7
  n0 --> n8
  classDef root stroke-width:3px
  classDef outside stroke-dasharray:4 3
  class n0 root
  class n3,n4,n8 outside
`, 0},
		// Callers point at the function asked about; an interface call goes
		// through the interface method's node.
		{[]string{"callers", "--format", "mermaid", "Store.Get"}, `flowchart LR
  n0["(*store.Store).Get"]
  n1["main.main"]
  n2{{"(store.Getter).Get"}}
  n1 --> n0
  n1 --> n2
  n2 -.-> n0
  classDef root stroke-width:3px
  class n0 root
`, 0},
		// # starts an entity in Mermaid.
		{[]string{"callers", "--format", "mermaid", "greet"}, `flowchart LR
  n0["main.greet"]
  n1["main.main$1"]
  n2["main.init@main.go#35;1"]
  n3["main.init@main.go#35;2"]
  n4["main.main"]
  n1 --> n0
  n2 --> n0
  n3 --> n0
  n4 -- "through a local" --> n0
  classDef root stroke-width:3px
  class n0 root
`, 0},
		// A call nothing resolves is a node of its own.
		{[]string{"callees", "--depth", "0", "--format", "mermaid", "Runner.locals"}, `flowchart LR
  n0["(*store.Runner).locals"]
  n1["(*store.Runner).locals$1"]
  n2["(*store.Runner).start"]
  n3["(*store.Runner).locals$2"]
  n4["store.handleLater"]
  n5(["(func value)"])
  n0 -- "through a local" --> n1
  n0 -- "through a local" --> n2
  n0 --> n3
  n3 -- "through a local" --> n1
  n3 -- "through a local" --> n2
  n0 -- "through a local" --> n4
  n0 -- "dynamic" --> n5
  classDef root stroke-width:3px
  class n0 root
`, 0},
		// DOT: full IDs as names; recursion is a loop, a repeat one more edge.
		{[]string{"callers", "--depth", "0", "--format", "dot", "leaf"}, `digraph callers {
  rankdir=LR;
  node [shape=box];
  "example.com/simple/store.leaf" [label="store.leaf", penwidth=2];
  "example.com/simple/store.walk" [label="store.walk"];
  "example.com/simple/store.visit" [label="store.visit"];
  "example.com/simple/store.walk" -> "example.com/simple/store.leaf";
  "example.com/simple/store.walk" -> "example.com/simple/store.walk";
  "example.com/simple/store.visit" -> "example.com/simple/store.leaf";
  "example.com/simple/store.walk" -> "example.com/simple/store.visit";
}
`, 0},
		// Package-level code, and a value stored.
		{[]string{"callers", "--depth", "0", "--format", "dot", "example.com/simple/store.nested$1$1"}, `digraph callers {
  rankdir=LR;
  node [shape=box];
  "example.com/simple/store.nested$1$1" [label="store.nested$1$1", penwidth=2];
  "example.com/simple/store.nested$1" [label="store.nested$1"];
  "example.com/simple/store" [label="store (package level)"];
  "example.com/simple/store.nested$1" -> "example.com/simple/store.nested$1$1";
  "example.com/simple/store" -> "example.com/simple/store.nested$1" [label="stored in store.nested"];
}
`, 0},
		// Nothing to draw: 1, as text.
		{[]string{"callees", "--format", "mermaid", "Fake.seal"}, "", 1},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			t.Parallel()
			stdout, stderr, code := run(t, append(tc.args, fixture)...)
			if code != tc.code || stdout != tc.want {
				t.Errorf("%v: exit %d, stdout:\n%s\nwant exit %d:\n%s\nstderr: %s", tc.args, code, stdout, tc.code, tc.want, stderr)
			}
		})
	}
}

// A field called through is one node; two calls to start (go, defer) are
// one edge.
func TestDiagramNodeAndEdgeOnce(t *testing.T) {
	t.Parallel()
	stdout, stderr, code := run(t, "callees", "--format", "dot", "Runner.closures", fixture)
	for _, line := range []string{
		`  "example.com/simple/store.Runner.OnDone" [label="store.Runner.OnDone", shape=ellipse];`,
		`  "(*example.com/simple/store.Runner).closures" -> "(*example.com/simple/store.Runner).start";`,
	} {
		if n := strings.Count(stdout, line+"\n"); code != 0 || n != 1 {
			t.Errorf("exit %d, %d lines %s, want 1; stdout:\n%s\nstderr: %s", code, n, line, stdout, stderr)
		}
	}
}

// Past 100 nodes, Mermaid gets a note on stderr; DOT doesn't.
func TestDiagramSizeNote(t *testing.T) {
	t.Parallel()
	for _, n := range []int{100, 101} {
		d := &diagram{index: map[string]int{}, seen: map[dedge]bool{}}
		for i := range n {
			d.node(fmt.Sprint(i), fmt.Sprint(i), funcNode, false)
		}
		for _, format := range []string{"mermaid", "dot"} {
			var out, errOut bytes.Buffer
			if err := d.write(&out, &errOut, format, "callees"); err != nil {
				t.Fatal(err)
			}
			note := strings.Contains(errOut.String(), fmt.Sprintf("%d nodes", n))
			if want := n > 100 && format == "mermaid"; note != want {
				t.Errorf("%d nodes, %s: stderr %q, want a note: %v", n, format, errOut.String(), want)
			}
		}
	}
}

func TestDiagramEscapes(t *testing.T) {
	t.Parallel()
	if got, want := mermaidText(`a "b" #1 <c> &d`), `a #quot;b#quot; #35;1 #lt;c#gt; #amp;d`; got != want {
		t.Errorf("mermaidText = %s, want %s", got, want)
	}
	if got, want := dotText(`a "b" \c`), `"a \"b\" \\c"`; got != want {
		t.Errorf("dotText = %s, want %s", got, want)
	}
}
