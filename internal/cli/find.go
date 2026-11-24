// find.go implements `grepo find`: loadModule → Index.Find → a line per
// declaration, most used first, or --json.

package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samippp/grepo/internal/query"
)

func newFindCmd(a *app) *cobra.Command {
	var (
		kinds   []string
		formats *formatFlag
	)

	cmd := &cobra.Command{
		Use:   "find <word> [path | github-link]",
		Short: "Declarations matching a word, most used first",
		Long: `Find prints the repo's declarations whose name has a word in it, most
used first, one per line at the declaration:

  path:line:col: name: source line  [kind, counts]

Funcs and methods rank by callers: direct calls, calls through a local
and uses as a func value. Calls through an interface the method's type
implements are counted apart, as "more via interfaces"; the two add up
to what callers prints. Fields, vars and consts rank by writes plus
reads, and types by the places code names them (refs). Ties go by
position; | sort gives position order back.

A word without a dot matches a declaration's own name: find load finds
Load and LoadModule, not everything in package loader. A word with a
dot matches the qualified name, (, ) and * removed: Context.Load or
caddytls. (everything in caddytls). Both ignore case. Closures and
types declared in functions aren't listed.

--kind keeps only some kinds: func, method, struct, interface, alias,
type (other named types), field, var, const.

Exit codes, as grep's: 0 something found, 1 nothing, 2 error.`,
		Example: `  grepo find Load
  grepo find --kind method,func Context.Load
  grepo find --json --kind struct Config https://github.com/caddyserver/caddy`,
		Args:        cobra.RangeArgs(1, 2),
		Annotations: map[string]string{quiet: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := formats.get(cmd)
			if err != nil {
				return err
			}
			var want []query.DeclKind
			for _, k := range kinds {
				if !slices.Contains(query.DeclKinds, query.DeclKind(k)) {
					return fmt.Errorf("--kind %s: want func, method, struct, interface, alias, type, field, var or const", k)
				}
				want = append(want, query.DeclKind(k))
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

			matches := rg.ix.Find(args[0], want)
			out := cmd.OutOrStdout()
			if format == "json" {
				err = writeJSON(out, p.findJSON(args[0], matches))
			} else {
				err = printFind(out, p, matches)
			}
			if err != nil {
				return err
			}
			if len(matches) == 0 {
				return errNoResults
			}
			return nil
		},
	}

	formats = addFormatFlags(cmd, "text", "json")
	cmd.Flags().StringSliceVar(&kinds, "kind", nil, "only these kinds, comma-separated or repeated")
	return cmd
}

// printFind prints a line per match: path:line:col: name: source line  [kind, counts].
func printFind(w io.Writer, p *printer, matches []query.Match) error {
	for _, m := range matches {
		text := p.path(m.Pos) + ": " + p.rg.ix.Short(m.ID)
		if line := p.line(m.Pos); line != "" {
			text += ": " + line
		}
		if _, err := fmt.Fprintf(w, "%s  [%s]\n", text, strings.Join(findTag(m), ", ")); err != nil {
			return err
		}
	}
	return nil
}

// findTag is a match's kind and counts: method, 4 callers, 9 more via interfaces.
func findTag(m query.Match) []string {
	tag := []string{string(m.Kind)}
	switch m.Kind {
	case query.FuncDecl, query.MethodDecl:
		tag = append(tag, count(m.Callers, "caller", "callers"))
		switch {
		case m.Via == 0:
		case m.Callers == 0:
			tag = append(tag, count(m.Via, "via an interface", "via interfaces"))
		default:
			tag = append(tag, count(m.Via, "more via an interface", "more via interfaces"))
		}
	case query.FieldDecl, query.VarDecl:
		tag = append(tag, count(m.Writes, "write", "writes"), count(m.Reads, "read", "reads"))
	case query.ConstDecl:
		tag = append(tag, count(m.Reads, "read", "reads"))
	default:
		tag = append(tag, count(m.Refs, "ref", "refs"))
		if m.Kind == query.InterfaceDecl {
			if m.Implementations > 0 {
				tag = append(tag, count(m.Implementations, "implementation", "implementations"))
			}
		} else if m.Methods > 0 {
			tag = append(tag, count(m.Methods, "method", "methods"))
		}
	}
	return tag
}

// count is "no callers", "1 caller" or "n callers".
func count(n int, one, many string) string {
	switch n {
	case 0:
		return "no " + many
	case 1:
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// findJSON is --json's output: IDs in full, positions as printed.
type findJSON struct {
	Base    string      `json:"base"` // the absolute directory positions are relative to
	Word    string      `json:"word"`
	Results []matchJSON `json:"results"`
}

// matchJSON is a declaration with its counts; only its kind's are set.
type matchJSON struct {
	Kind query.DeclKind `json:"kind"`
	ID   string         `json:"id"`
	Pos  string         `json:"pos"`
	Line string         `json:"line,omitempty"`
	Uses int            `json:"uses"` // what results are ranked by

	Callers *int `json:"callers,omitempty"`
	Via     *int `json:"via,omitempty"` // calls through an interface

	Writes *int `json:"writes,omitempty"`
	Reads  *int `json:"reads,omitempty"`

	Refs            *int `json:"refs,omitempty"`
	Methods         *int `json:"methods,omitempty"`
	Implementations *int `json:"implementations,omitempty"`
}

func (p *printer) findJSON(word string, matches []query.Match) findJSON {
	out := findJSON{Base: filepath.ToSlash(p.rg.base), Word: word, Results: []matchJSON{}}
	for _, m := range matches {
		js := matchJSON{Kind: m.Kind, ID: m.ID, Pos: p.path(m.Pos), Line: p.line(m.Pos), Uses: m.Uses}
		switch m.Kind {
		case query.FuncDecl, query.MethodDecl:
			js.Callers, js.Via = &m.Callers, &m.Via
		case query.FieldDecl, query.VarDecl, query.ConstDecl:
			js.Writes, js.Reads = &m.Writes, &m.Reads
		case query.InterfaceDecl:
			js.Refs, js.Implementations = &m.Refs, &m.Implementations
		default:
			js.Refs, js.Methods = &m.Refs, &m.Methods
		}
		out.Results = append(out.Results, js)
	}
	return out
}
