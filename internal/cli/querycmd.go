// querycmd.go is what the query commands share: a module's graph indexed for
// queries, and positions printed grep-style. For now queries load from
// source, without the store (docs/architecture.md, Queries).

package cli

import (
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
		return "", fmt.Errorf("--format %s: %s prints %s", f.format, cmd.Name(), strings.Join(f.offered, " or "))
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
	return reportedError{err}
}
