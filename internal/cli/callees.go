// callees.go implements `grepo callees`: Index.CalleeTree, printed as an
// indented list or --json.

package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samippp/grepo/internal/graph"
	"github.com/samippp/grepo/internal/query"
)

func newCalleesCmd(a *app) *cobra.Command {
	var (
		direct  bool
		depth   int
		formats *formatFlag
	)

	cmd := &cobra.Command{
		Use:   "callees <function> [path | github-link]",
		Short: "Everything a function calls, through interfaces too, as a tree with --depth",
		Long: `Callees prints every site in a function that reaches another: its calls,
and the functions it passes or stores as values. One site per line,
grep-style, by position:

  path:line:col: function reached: source line  [how]

[interface] is a call through an interface, [through a local] one through
a local holding the function (a line per function assigned to it), and
[dynamic] a call through a func value nothing resolves. [passed to f] and
[stored in field] are functions used as values, which may be called later.
[go] and [defer] mark those statements.

--depth n follows n levels (0: all the way), indenting each level after
the position. An interface call's methods are under it, at their
declarations, as [implements I]. A function already expanded is marked
[see above], and one calling back into the path above it [recursive].
Functions outside the repo aren't followed.

--format mermaid or dot draws the tree as a graph, to paste into an
issue or PR: a node per function, edges from caller to callee, labeled
when not a direct call. An interface call goes to the interface
method, with dotted edges to its implementations. Past 100 nodes, a
note on stderr suggests a smaller --depth.

The function is named as for callers. The whole module is loaded from
source each time, for now.

Exit codes, as grep's: 0 sites found, 1 none, 2 error.`,
		Example: `  grepo callees main
  grepo callees --depth 2 '(*Server).ServeHTTP' ./modules/caddyhttp
  grepo callees --depth 0 --direct --json caddy.Load
  grepo callees --depth 2 --format mermaid main`,
		Args:        cobra.RangeArgs(1, 2),
		Annotations: map[string]string{quiet: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := formats.get(cmd)
			if err != nil {
				return err
			}
			if depth < 0 {
				return fmt.Errorf("--depth %d: want 0 (all the way) or more", depth)
			}
			where := "."
			if len(args) == 2 {
				where = args[1]
			}
			m, err := loadModule(cmd.Context(), a.log, where, loadOptions{})
			if err != nil {
				return err
			}
			rg := newRepoGraph(m)
			p := newPrinter(rg)

			fn, err := rg.ix.Func(args[0], rg.prefer)
			if amb, ok := errors.AsType[*query.AmbiguousError](err); ok {
				return p.ambiguous(cmd.ErrOrStderr(), amb)
			} else if err != nil {
				return err
			}

			nodes := rg.ix.CalleeTree(fn, depth, direct)
			out := cmd.OutOrStdout()
			switch format {
			case "json":
				err = writeJSON(out, calleesJSON{Base: filepath.ToSlash(rg.base), Function: p.function(fn), Callees: p.nodesJSON(nodes)})
			case "mermaid", "dot":
				if len(nodes) > 0 {
					err = calleeDiagram(p, fn, nodes).write(out, cmd.ErrOrStderr(), format, "callees")
				}
			default:
				err = printCallees(out, p, nodes, 0)
			}
			if err != nil {
				return err
			}
			if len(nodes) == 0 {
				return errNoResults
			}
			return nil
		},
	}

	formats = addFormatFlags(cmd, "text", "json", "mermaid", "dot")
	cmd.Flags().IntVar(&depth, "depth", 1, "levels to follow; 0 for all the way")
	cmd.Flags().BoolVar(&direct, "direct", false, "only static calls: no interface calls, locals, func values")
	return cmd
}

// calleesJSON is --json's output.
type calleesJSON struct {
	Base     string         `json:"base"`
	Function graph.Function `json:"function"`
	Callees  []siteJSON     `json:"callees"`
}

// printCallees prints a line per node, children indented.
func printCallees(w io.Writer, p *printer, nodes []*query.Node, level int) error {
	for _, n := range nodes {
		if _, err := fmt.Fprintln(w, calleeLine(p, n, level)); err != nil {
			return err
		}
		if err := printCallees(w, p, n.Children, level+1); err != nil {
			return err
		}
	}
	return nil
}

func calleeLine(p *printer, n *query.Node, level int) string {
	ix := p.rg.ix
	name := ix.Short(n.Target)
	if n.Kind == query.Dynamic {
		name = "(func value)"
		if n.Call.Through != "" { // r.onDone()
			name = ix.Short(n.Call.Through)
		}
	}
	text := p.path(n.Pos) + ": " + strings.Repeat("  ", level) + name
	if n.Kind != query.Implementation { // a declaration
		if line := p.line(n.Pos); line != "" {
			text += ": " + line
		}
	}

	var how []string
	if n.Call != nil && n.Call.Stmt != "" && n.Kind != query.Value {
		how = append(how, n.Call.Stmt) // go, defer
	}
	switch n.Kind {
	case query.Interface:
		how = append(how, "interface")
	case query.Local:
		how = append(how, "through a local")
	case query.Dynamic:
		how = append(how, "dynamic")
	case query.Value:
		how = append(how, p.valueTag(n.Site))
	case query.Implementation:
		how = append(how, "implements "+ix.Short(n.Via))
	}
	if n.Seen {
		how = append(how, "see above")
	}
	if n.Recursive {
		how = append(how, "recursive")
	}
	if len(how) > 0 {
		text += "  [" + strings.Join(how, ", ") + "]"
	}
	return text
}
