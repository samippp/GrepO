// state.go implements `grepo state`: loadModule → Index.Target → a line per
// field of a struct (Index.Fields), or a line per site of a field or var
// (Index.Accesses), or --json.

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

func newStateCmd(a *app) *cobra.Command {
	var (
		reads   bool
		formats *formatFlag
	)

	cmd := &cobra.Command{
		Use:   "state <struct|field|var> [path | github-link]",
		Short: "Who writes a struct's fields or a var, by kind, with a role where the writes settle one",
		Long: `State prints who changes a struct's fields or a package var.

A struct gives a line per field, at the field, with its writes by kind:

  path:line:col: field: source line  [role: counts]

The role is there only when the writes settle it: "no writes found",
"set at construction" (construct writes only: T{f: v}) or "counter" (a
number, updates only: x++, x += n). Escapes (&x, s.mu.Lock(), a map or
slice handed on) are counted apart: a change can happen where access
went. Writes the graph can't see, such as encoding/json filling a
config struct, aren't counted, hence "no writes found".

A field or var gives a line per site that writes it or hands out access
to it, by position, tagged with the kind: assign, update, content (into
what it holds: x[k] = v), construct, escape. --reads adds the reads.

Names are as callers takes them, qualified as much as needed: Store,
Store.users, store.Store.users, a path suffix. An alias of a struct is
the struct. Only the repo's types, fields and vars, for now.

Exit codes, as grep's: 0 something found, 1 nothing, 2 error.`,
		Example: `  grepo state Store
  grepo state Store.users
  grepo state --reads --json defaultStore ./store`,
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

			t, err := rg.ix.Target(args[0], rg.prefer)
			if amb, ok := errors.AsType[*query.AmbiguousError](err); ok {
				return p.ambiguous(cmd.ErrOrStderr(), amb)
			} else if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			var found bool
			switch t.Kind {
			case query.StructDecl:
				if reads {
					return errors.New("--reads takes a field or var, not a struct")
				}
				fields := rg.ix.Fields(t.Type)
				found = len(fields) > 0
				if format == "json" {
					err = writeJSON(out, p.typeStateJSON(t.Type, fields))
				} else {
					err = printFields(out, p, fields)
				}
			case query.FieldDecl, query.VarDecl, query.ConstDecl:
				sites := rg.ix.Accesses(t.ID, reads)
				found = len(sites) > 0
				if format == "json" {
					err = writeJSON(out, p.accessesJSON(t, sites))
				} else {
					err = printAccesses(out, p, sites)
				}
			default:
				return fmt.Errorf("%s is %s; state takes a struct, a field or a package var", rg.ix.Short(t.ID), kindPhrase[t.Kind])
			}
			if err != nil {
				return err
			}
			if !found {
				return errNoResults
			}
			return nil
		},
	}

	formats = addFormatFlags(cmd, "text", "json")
	cmd.Flags().BoolVar(&reads, "reads", false, "a field or var's reads too")
	return cmd
}

var kindPhrase = map[query.DeclKind]string{query.InterfaceDecl: "an interface", query.AliasDecl: "an alias", query.TypeDecl: "a named type"}

// printFields prints a line per field: path:line:col: field: source line  [role: counts].
func printFields(w io.Writer, p *printer, fields []query.FieldState) error {
	for _, f := range fields {
		text := p.path(f.Field.Pos) + ": " + p.rg.ix.Short(f.Field.ID)
		if line := p.line(f.Field.Pos); line != "" {
			text += ": " + line
		}
		if _, err := fmt.Fprintf(w, "%s  [%s]\n", text, fieldTag(f)); err != nil {
			return err
		}
	}
	return nil
}

// writeOrder is the order write counts are listed in.
var writeOrder = []struct {
	kind      graph.AccessKind
	one, many string
}{
	{graph.AssignAccess, "assign", "assigns"},
	{graph.ConstructAccess, "construct", "constructs"},
	{graph.ContentAccess, "content write", "content writes"},
	{graph.UpdateAccess, "update", "updates"},
}

// fieldTag is a field's role and counts: "counter: 3 updates, 1 escape".
func fieldTag(f query.FieldState) string {
	var counts []string
	for _, k := range writeOrder {
		if n := f.Writes[k.kind]; n > 0 {
			counts = append(counts, count(n, k.one, k.many))
		}
	}
	if f.Escapes > 0 {
		counts = append(counts, count(f.Escapes, "escape", "escapes"))
	}
	switch {
	case f.Role == query.NoWrites:
		return strings.Join(append([]string{string(f.Role)}, counts...), ", ")
	case f.Role != "":
		return string(f.Role) + ": " + strings.Join(counts, ", ")
	}
	return strings.Join(counts, ", ")
}

// printAccesses prints a line per site: path:line:col: from: source line  [kind].
func printAccesses(w io.Writer, p *printer, sites []query.Site) error {
	for _, s := range sites {
		text := p.path(s.Pos) + ": " + p.from(s.From)
		if line := p.line(s.Pos); line != "" {
			text += ": " + line
		}
		if _, err := fmt.Fprintf(w, "%s  [%s]\n", text, s.Access.Kind); err != nil {
			return err
		}
	}
	return nil
}

// typeStateJSON is --json for a struct: the type, then its fields' counts.
type typeStateJSON struct {
	Base   string      `json:"base"`
	Type   graph.Type  `json:"type"`
	Fields []fieldJSON `json:"fields"`
}

type fieldJSON struct {
	graph.Field
	Line    string                   `json:"line,omitempty"`
	Role    query.Role               `json:"role,omitempty"`
	Writes  map[graph.AccessKind]int `json:"writes"` // by kind, no zeros
	Escapes int                      `json:"escapes"`
}

func (p *printer) typeStateJSON(t *graph.Type, fields []query.FieldState) typeStateJSON {
	out := typeStateJSON{Base: filepath.ToSlash(p.rg.base), Type: *t, Fields: []fieldJSON{}}
	out.Type.Pos = p.path(t.Pos)
	out.Type.Fields = nil // they're under fields
	for _, f := range fields {
		js := fieldJSON{Field: *f.Field, Line: p.line(f.Field.Pos), Role: f.Role, Writes: f.Writes, Escapes: f.Escapes}
		js.Pos = p.path(f.Field.Pos)
		out.Fields = append(out.Fields, js)
	}
	return out
}

// accessesJSON is --json for a field or var: it, then its sites.
type accessesJSON struct {
	Base  string          `json:"base"`
	Field *graph.Field    `json:"field,omitempty"`
	Var   *graph.Variable `json:"var,omitempty"` // consts too
	Sites []siteJSON      `json:"sites"`
}

func (p *printer) accessesJSON(t query.Target, sites []query.Site) accessesJSON {
	out := accessesJSON{Base: filepath.ToSlash(p.rg.base), Sites: []siteJSON{}}
	if t.Field != nil {
		f := *t.Field
		f.Pos = p.path(f.Pos)
		out.Field = &f
	} else {
		v := *t.Var
		v.Pos = p.path(v.Pos)
		out.Var = &v
	}
	for _, s := range sites {
		out.Sites = append(out.Sites, p.siteJSON(s))
	}
	return out
}
