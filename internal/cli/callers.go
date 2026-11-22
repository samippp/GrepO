// callers.go implements `grepo callers`: loadModule → Index.Func →
// Index.Callers → one line per site, or --json.

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/samippp/grepo/internal/graph"
	"github.com/samippp/grepo/internal/query"
)

func newCallersCmd(a *app) *cobra.Command {
	var (
		direct  bool
		formats *formatFlag
	)

	cmd := &cobra.Command{
		Use:   "callers <function> [path | github-link]",
		Short: "Every site that can call a function, through interfaces too",
		Long: `Callers prints every site that can call a function: direct calls, calls
through an interface its type implements, and places it's passed or stored
as a func value. One site per line, grep-style:

  path:line:col: calling function: source line  [how]

Direct calls come first, then [via Interface], then [through a local]
(f := s.Get; f()), then [passed to f] and [stored in field]; each group
by position. Paths are relative to the
current directory.

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
  grepo callers --json caddy.Load https://github.com/caddyserver/caddy`,
		Args:        cobra.RangeArgs(1, 2),
		Annotations: map[string]string{quiet: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := formats.get(cmd)
			if err != nil {
				return err
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

			sites := rg.ix.Callers(fn)
			if direct {
				var kept []query.Site
				for _, s := range sites {
					if s.Kind == query.Direct {
						kept = append(kept, s)
					}
				}
				sites = kept
			}

			out := cmd.OutOrStdout()
			if format == "json" {
				err = printCallersJSON(out, p, fn, sites)
			} else {
				err = printCallers(out, p, sites)
			}
			if err != nil {
				return err
			}
			if len(sites) == 0 {
				return errNoResults
			}
			return nil
		},
	}

	formats = addFormatFlags(cmd, "text", "json")
	cmd.Flags().BoolVar(&direct, "direct", false, "only direct calls: no calls through interfaces or func values")
	return cmd
}

// printCallers prints one site per line: path:line:col: from: source line  [how].
func printCallers(w io.Writer, p *printer, sites []query.Site) error {
	ix := p.rg.ix
	for _, s := range sites {
		from := ix.Short(s.From)
		if ix.IsPackage(s.From) {
			from += " (package level)"
		}
		text := p.path(s.Pos) + ": " + from
		if line := p.line(s.Pos); line != "" {
			text += ": " + line
		}
		switch {
		case s.Kind == query.Interface:
			text += "  [via " + ix.Short(s.Via) + "]"
		case s.Kind == query.Local:
			text += "  [through a local]"
		case s.Kind == query.Value && s.Access != nil:
			text += "  [stored in " + ix.Short(s.Via) + "]"
		case s.Kind == query.Value && s.Via == "":
			text += "  [passed to a func value]"
		case s.Kind == query.Value:
			text += "  [passed to " + ix.Short(s.Via) + "]"
		}
		if _, err := fmt.Fprintln(w, text); err != nil {
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

type siteJSON struct {
	Kind query.Kind `json:"kind"` // direct, interface, local or value
	Pos  string     `json:"pos"`  // path:line:col, relative to the current directory
	From string     `json:"from"` // the function the site is in, or the package for package-level code
	// FromDecl is the declaration a closure From is written in: a declared
	// function, or the package var it initializes.
	FromDecl string `json:"fromDecl,omitempty"`
	Via      string `json:"via,omitempty"`  // interface: its ID; value: the function it's passed to, or the field or var it's stored in
	Line     string `json:"line,omitempty"` // the source line, trimmed

	// The call at the site, if any.
	Callee   string         `json:"callee,omitempty"`
	CallKind graph.CallKind `json:"callKind,omitempty"`
	Args     []graph.Arg    `json:"args,omitempty"`
	Through  string         `json:"through,omitempty"`
	Stmt     string         `json:"stmt,omitempty"`
	Funcs    []string       `json:"funcs,omitempty"` // local: every function assigned to the local called

	Access graph.AccessKind `json:"access,omitempty"` // a value stored: assign or construct
}

func printCallersJSON(w io.Writer, p *printer, fn *graph.Function, sites []query.Site) error {
	out := callersJSON{Base: filepath.ToSlash(p.rg.base), Function: *fn, Sites: []siteJSON{}}
	if fn.Pos != "" { // "" outside the repo
		out.Function.Pos = p.path(fn.Pos)
	}
	for _, s := range sites {
		js := siteJSON{Kind: s.Kind, Pos: p.path(s.Pos), From: s.From, FromDecl: p.rg.ix.Decl(s.From), Via: s.Via, Line: p.line(s.Pos)}
		if c := s.Call; c != nil {
			js.Callee, js.CallKind, js.Args, js.Through, js.Stmt, js.Funcs = c.Callee, c.CallKind, c.Args, c.Through, c.Stmt, c.Funcs
		}
		if s.Access != nil {
			js.Access = s.Access.Kind
		}
		out.Sites = append(out.Sites, js)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
