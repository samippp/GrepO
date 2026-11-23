// callers.go implements `grepo callers`: loadModule → Index.Func →
// Index.CallerTree → a line per site, children indented, or --json.

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

func newCallersCmd(a *app) *cobra.Command {
	var (
		direct  bool
		depth   int
		formats *formatFlag
	)

	cmd := &cobra.Command{
		Use:   "callers <function> [path | github-link]",
		Short: "Every site that can call a function, through interfaces too, as a tree with --depth",
		Long: `Callers prints every site that can call a function: direct calls, calls
through an interface its type implements, and places it's passed or stored
as a func value. One site per line, grep-style:

  path:line:col: calling function: source line  [how]

Direct calls come first, then [via Interface], then [through a local]
(f := s.Get; f()), then [passed to f] and [stored in field]; each group
by position. Paths are relative to the current directory.

--depth n follows n levels (0: all the way): under each site, the sites
that can call the function it's in, indented after the position. A
function already expanded is marked [see above], and one in the path
above [recursive]. Package-level code isn't followed. --direct applies
at every level.

The function is named as in Go, qualified as much as needed: Get,
Store.Get, (*Store).Get, store.Store.Get, or a path suffix such as
caddyhttp/reverseproxy.Handler.ServeHTTP. When several match, the one in
the package in the current directory (or the path given) wins; otherwise
they're listed, and the command exits with 2. Functions outside the repo
work too (os.Exit, sql.DB.Exec), but a repo function of the same name
wins: qualify the outside one.

The whole module is loaded from source each time, for now.

Exit codes, as grep's: 0 sites found, 1 none, 2 error.`,
		Example: `  grepo callers Store.Get
  grepo callers '(*Server).ServeHTTP' ./modules/caddyhttp
  grepo callers --depth 3 --direct caddy.Load
  grepo callers --json caddy.Load https://github.com/caddyserver/caddy`,
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

			nodes := rg.ix.CallerTree(fn, depth, direct)
			out := cmd.OutOrStdout()
			if format == "json" {
				err = writeJSON(out, callersJSON{Base: filepath.ToSlash(rg.base), Function: p.function(fn), Sites: p.nodesJSON(nodes)})
			} else {
				err = printCallers(out, p, nodes, 0)
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

	formats = addFormatFlags(cmd, "text", "json")
	cmd.Flags().IntVar(&depth, "depth", 1, "levels to follow; 0 for all the way")
	cmd.Flags().BoolVar(&direct, "direct", false, "only direct calls, at every level: no calls through interfaces or func values")
	return cmd
}

// printCallers prints a line per node, path:line:col: from: source line  [how],
// children indented after the position.
func printCallers(w io.Writer, p *printer, nodes []*query.Node, level int) error {
	ix := p.rg.ix
	for _, n := range nodes {
		from := ix.Short(n.From)
		if ix.IsPackage(n.From) {
			from += " (package level)"
		}
		text := p.path(n.Pos) + ": " + strings.Repeat("  ", level) + from
		if line := p.line(n.Pos); line != "" {
			text += ": " + line
		}
		var how []string
		switch n.Kind {
		case query.Interface:
			how = append(how, "via "+ix.Short(n.Via))
		case query.Local:
			how = append(how, "through a local")
		case query.Value:
			how = append(how, p.valueTag(n.Site))
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
		if _, err := fmt.Fprintln(w, text); err != nil {
			return err
		}
		if err := printCallers(w, p, n.Children, level+1); err != nil {
			return err
		}
	}
	return nil
}

// callersJSON is --json's output: IDs in full, positions as printed.
type callersJSON struct {
	Base     string         `json:"base"` // the absolute directory positions are relative to
	Function graph.Function `json:"function"`
	Sites    []siteJSON     `json:"sites"`
}
