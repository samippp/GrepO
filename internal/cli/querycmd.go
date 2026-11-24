// querycmd.go is what the query commands share: a module's graph indexed for
// queries, and positions printed grep-style. For now queries load from
// source, without the store (docs/architecture.md, Queries).

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samippp/grepo/internal/graph"
	"github.com/samippp/grepo/internal/query"
)

// quiet is the annotation a query command sets to hide Info logs unless -v.
const quiet = "quiet"

// formatFlag is a query command's --format, with --json short for --format
// json (docs/output.md, Formats).
type formatFlag struct {
	format  string
	json    bool
	offered []string // what the command prints, text first
}

// addFormatFlags adds --format and --json to cmd, which prints the formats
// offered.
func addFormatFlags(cmd *cobra.Command, offered ...string) *formatFlag {
	f := &formatFlag{offered: offered}
	cmd.Flags().StringVar(&f.format, "format", offered[0], "output format: "+strings.Join(offered, ", "))
	cmd.Flags().BoolVar(&f.json, "json", false, "short for --format json")
	cmd.MarkFlagsMutuallyExclusive("format", "json")
	return f
}

// get is the format asked for, or an error naming what cmd prints.
func (f *formatFlag) get(cmd *cobra.Command) (string, error) {
	if f.json {
		return "json", nil
	}
	if !slices.Contains(f.offered, f.format) {
		n := len(f.offered)
		list := strings.Join(f.offered[:n-1], ", ") + " or " + f.offered[n-1]
		return "", fmt.Errorf("--format %s: %s prints %s", f.format, cmd.Name(), list)
	}
	return f.format, nil
}

// repoGraph is a repo loaded for queries.
type repoGraph struct {
	ix     *query.Index
	root   string // absolute: the module root
	prefer string // import path of the package in the directory asked about, "" if none
	base   string // printed paths are relative to it
}

// newRepoGraph indexes m for queries. Names prefer the package in the
// directory asked about.
func newRepoGraph(m *module) *repoGraph {
	rg := &repoGraph{ix: query.New(m.pg, m.cg), root: m.res.Dir, base: m.res.Dir}
	for _, p := range m.res.Packages {
		if p.Dir == m.dir {
			rg.prefer = p.PkgPath
		}
	}
	// A local repo's paths are relative to the working directory, so they're
	// clickable; a clone's are the repo's own.
	if m.repo.URL == "" {
		if wd, err := os.Getwd(); err == nil {
			rg.base = wd
		}
	}
	return rg
}

// printer prints positions and the source lines at them.
type printer struct {
	rg    *repoGraph
	files map[string][]string // lines, by repo-relative file
}

func newPrinter(rg *repoGraph) *printer {
	return &printer{rg: rg, files: map[string][]string{}}
}

// path is a graph position, relative to the root, made relative to rg.base.
func (p *printer) path(pos string) string {
	file, line, col := graph.SplitPos(pos)
	abs := filepath.Join(p.rg.root, filepath.FromSlash(file))
	if rel, err := filepath.Rel(p.rg.base, abs); err == nil {
		abs = rel
	}
	if line == 0 {
		return abs
	}
	abs += ":" + strconv.Itoa(line)
	if col == 0 {
		return abs
	}
	return abs + ":" + strconv.Itoa(col)
}

// line is the trimmed source line at pos, or "" if it can't be read.
func (p *printer) line(pos string) string {
	file, n, _ := graph.SplitPos(pos)
	lines, ok := p.files[file]
	if !ok {
		data, err := os.ReadFile(filepath.Join(p.rg.root, filepath.FromSlash(file)))
		if err == nil {
			lines = strings.Split(string(data), "\n")
		}
		p.files[file] = lines
	}
	if n < 1 || n > len(lines) {
		return ""
	}
	return strings.TrimSpace(lines[n-1])
}

// ambiguous lists err's candidates, each with a name that picks it: its
// short name, or its ID when two short names collide. It returns the
// error to exit with.
func (p *printer) ambiguous(w io.Writer, err *query.AmbiguousError) error {
	short := map[string]int{}
	for _, f := range err.Candidates {
		short[p.rg.ix.Short(f.ID)]++
	}
	fmt.Fprintf(w, "%s matches %d functions; name one of them:\n", err.Name, len(err.Candidates))
	for _, f := range err.Candidates {
		name := p.rg.ix.Short(f.ID)
		if short[name] > 1 {
			name = f.ID
		}
		if f.Pos == "" {
			fmt.Fprintf(w, "%s (outside the repo)\n", name)
		} else {
			fmt.Fprintf(w, "%s: %s\n", p.path(f.Pos), name)
		}
	}
	fmt.Fprintf(w, "grepo find %s ranks them by use\n", query.CleanName(err.Name))
	return reportedError{err}
}

// valueTag is "passed to f" or "stored in field".
func (p *printer) valueTag(s query.Site) string {
	switch {
	case s.Access != nil:
		return "stored in " + p.rg.ix.Short(s.Via)
	case s.Via == "":
		return "passed to a func value"
	default:
		return "passed to " + p.rg.ix.Short(s.Via)
	}
}

// function is fn for --json, its position as printed.
func (p *printer) function(fn *graph.Function) graph.Function {
	out := *fn
	if fn.Pos != "" { // "" outside the repo
		out.Pos = p.path(fn.Pos)
	}
	return out
}

// siteJSON is a site in every command's --json: full IDs, printed positions.
type siteJSON struct {
	Kind query.Kind `json:"kind"`
	Pos  string     `json:"pos"`  // path:line:col, relative to base
	From string     `json:"from"` // the function the site is in, or its package
	// FromDecl: Index.Decl(From), for closures.
	FromDecl string `json:"fromDecl,omitempty"`
	// Via: the interface, or the function or field a value goes to.
	Via    string `json:"via,omitempty"`
	Target string `json:"target,omitempty"` // callees: the function reached
	Line   string `json:"line,omitempty"`

	// The call at the site, if any.
	Callee   string         `json:"callee,omitempty"`
	CallKind graph.CallKind `json:"callKind,omitempty"`
	Args     []graph.Arg    `json:"args,omitempty"`
	Through  string         `json:"through,omitempty"`
	Stmt     string         `json:"stmt,omitempty"`
	Funcs    []string       `json:"funcs,omitempty"` // a local's functions

	Access graph.AccessKind `json:"access,omitempty"` // a value stored

	// Trees (--depth).
	Children  []siteJSON `json:"children,omitempty"`
	Seen      bool       `json:"seen,omitempty"`
	Recursive bool       `json:"recursive,omitempty"`
}

func (p *printer) siteJSON(s query.Site) siteJSON {
	js := siteJSON{Kind: s.Kind, Pos: p.path(s.Pos), From: s.From, FromDecl: p.rg.ix.Decl(s.From), Via: s.Via, Target: s.Target}
	if s.Kind != query.Implementation { // a declaration
		js.Line = p.line(s.Pos)
	}
	if c := s.Call; c != nil {
		js.Callee, js.CallKind, js.Args, js.Through, js.Stmt, js.Funcs = c.Callee, c.CallKind, c.Args, c.Through, c.Stmt, c.Funcs
	}
	if s.Access != nil {
		js.Access = s.Access.Kind
	}
	return js
}

// nodesJSON is a tree for --json.
func (p *printer) nodesJSON(nodes []*query.Node) []siteJSON {
	out := []siteJSON{}
	for _, n := range nodes {
		js := p.siteJSON(n.Site)
		js.Seen, js.Recursive = n.Seen, n.Recursive
		if len(n.Children) > 0 {
			js.Children = p.nodesJSON(n.Children)
		}
		out = append(out, js)
	}
	return out
}

// writeJSON writes v indented.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
