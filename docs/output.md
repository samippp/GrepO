# grepo output design

How every query command prints its answer: `callers` (done), `callees`, `find`, `state`, `pkg`, `entry`, `impact`, and `grepo mcp`. Commands get built to fit these rules, so later ones don't need their own conventions. `analyze` stays as it is: it dumps the whole graph, and isn't a query.

Status: proposal. Section 9 lists what changes before the next command, and section 10 the decisions still open.

Examples use the test fixture (`internal/graph/testdata/simple`). Ones marked *sketch* show the shape, not final wording.

## 1. Rules for every command

1. **Every line starts with a location**, `path:line:col: `, as compiler errors, `go vet` and `rg --vimgrep` do. Editors and terminals make it clickable, and vim reads the output as a quickfix list (`grepo callers Get | vim -q -` or `:cexpr system(...)`). A package's location is its directory (`store/`).
2. **A line stands alone.** There are no headers, blank-line groups or "continued from above" lines. How a result relates to the question goes in a tag at the end (`[via store.Getter]`), so `| grep store/`, `| sort` and `| wc -l` still mean something.
3. **A summary is located where its subject is declared.** A field's writers are counted on a line at the field, and a function's callers on a line at the function. Drilling down means asking about the thing on that line.
4. **Text uses short names; JSON uses full IDs.** Text names are as Go code writes them (`(*store.Store).Get`, from `Index.Short`). JSON has IDs (`(*example.com/simple/store.Store).Get`), which you can pass back as names.
5. **stdout holds results only.** Logs, the list for an ambiguous name, and notes ("partial: dependencies still loading", "graph has 400 nodes") go to stderr.
6. **Exit codes are grep's:** 0 found something, 1 found nothing, 2 an error, including an ambiguous name.
7. **Paths are relative to the current directory** for a local repo, and to the repo root for a clone. JSON says what they're relative to (`base`).
8. **Nothing is truncated or paged.** `| head` does that. Depth (section 4) is what bounds trees. Color may come later, only on a terminal, as grep's `--color=auto`.

## 2. The line

```
path:line:col: name: source line  [how]
```

| Part | What it is |
|---|---|
| `path:line:col` | the result's position; `col` is the 1-based byte column, as `go/token`, gopls and vim count it |
| `name` | what's at the position: for a site, the function it's in (`main.main`, `main.main$1` for a closure); for a declaration, what's declared |
| `source line` | the trimmed line at the position, read when printed; left out when the file can't be read |
| `[how]` | how the result relates to what was asked, if not plainly; one tag, at most a few words |

A call site's position is the start of its call expression, and a declaration's is its name. This is the same rule the graph uses, so text, JSON and the graph agree.

Closures keep their IDs (`doActiveHealthCheck$1`), not their enclosing function's. The source line already gives the context, and the ID can be asked about. JSON adds the enclosing declared function (section 6).

## 3. Formats

`--format text|json|mermaid|dot`, with `--json` short for `--format json`. Every command has `text` and `json`. Only commands whose answer is a graph have `mermaid` and `dot`: `pkg`, `callers`/`callees` with `--depth`, and `impact`. Asking another command for them exits with 2.

## 4. Trees: `--depth`

`callers` and `callees` print one level by default: flat, as `callers` does now. `--depth n` follows n levels, and `--depth 0` goes all the way. Each line is still position-first. The tree is shown by indenting the name, two spaces per level, so quickfix and grep keep working:

```
$ grepo callees --depth 2 main.main
main.go:16:7: store.New: s := store.New()
main.go:17:7: (*store.Store).Get: u := s.Get(1)
store/store.go:24:2:   (*sync.Mutex).Lock: s.mu.Lock()
store/store.go:25:8:   (*sync.Mutex).Unlock: defer s.mu.Unlock()  [defer]
main.go:20:2: store.Getter.Get: g.Get(2)  [interface]
store/store.go:23:17:   (*store.Store).Get  [implements store.Getter]  [see above]
main.go:23:2: main.greet: f(u.Name)  [through a local]
...
```

- **Interface calls** get their implementations as children. Each is located at its declaration and tagged `[implements I]`, and each expands in turn.
- **Repeats:** a function already expanded is printed again with `[see above]`, and isn't expanded twice. A call back into the path above is `[recursive]`.
- **Leaves:** functions outside the repo aren't expanded, and calls to them show as leaves.
- **`--direct`** drops the interface, local and value edges at every level.

A drawn tree (`├─`) was considered and rejected: it breaks rule 1, and quickfix can't read it.

## 5. Per command

Only `callers` exists. The others are *sketches* of how they fit the rules. Each command's own design settles its details.

**`callers <func>`**: as now, plus columns.
```
main.go:17:7: main.main: u := s.Get(1)
main.go:20:2: main.main: g.Get(2)  [via store.Getter]
```

**`callees <func>`**: the calls a function makes, by position, one line per call site. Within a site, a dynamic call with several candidates (`Funcs`) gives a line per candidate, tagged `[through a local]`. A call nothing resolves is tagged `[dynamic]`.

**`find <word>`** *sketch*: declarations matching a word, ranked by how much they're used, most first.
```
store/store.go:23:17: (*store.Store).Get  [method, 2 callers]
store/store.go:12:6: store.Getter  [interface, 1 implementation]
```

**`state <type|field|var>`** *sketch*: rule 3 in action. A type gives a line per field, at the field, with its role and a count of writes by kind. A field or var gives a line per write site.
```
$ grepo state Store
store/store.go:17:2: Store.mu: mu sync.Mutex  [no writes found]
store/store.go:18:2: Store.users: users map[int]model.User  [config: 1 construct]
$ grepo state Store.users
store/store.go:21:35: store.New: func New() *Store { return &Store{users: map[int]model.User{}} }  [construct]
```

**`pkg [path]`** *sketch*: with no path, packages ranked by how central they are, located at their directories. With a path, the package's facts, one per line, each tagged with what it is.
```
$ grepo pkg store
model/: example.com/simple/model  [imports]
./: example.com/simple  [imported by]
store/store.go:23:17: (*store.Store).Get  [API: 2 callers]
store/store.go:18:2: Store.users  [state: 1 write]
```

**`entry`** *sketch*: where execution starts. `main` and `init` are located at their declarations. Handlers and commands are located at their registration, which is where the route or command name is.
```
main.go:15:6: main.main  [main]
store/closures.go:40:2: handle: http.HandleFunc("/a", handle)  [HTTP handler]
```

**`impact <symbol>`** *sketch*: by default, rolled up into a line per package with counts. `--sites` lists each site, as `callers` does.

## 6. JSON

`--json` is the contract with scripts, agents and `grepo mcp`. It is one object per run, indented:

```json
{
  "base": "/home/u/src/simple",
  "function": { "id": "(*example.com/simple/store.Store).Get", "pos": "store/store.go:23:17", ... },
  "sites": [
    { "kind": "direct", "pos": "main.go:17:7", "from": "example.com/simple.main", "line": "u := s.Get(1)", ... }
  ]
}
```

- **`base`** is the absolute directory that positions are relative to, so a program in another directory can open the files.
- **The subject** is the node asked about, whole (`function`, `field`, `package`), under a key naming its kind.
- **The results** are a list under a key naming what they are (`sites`, `calls`, `fields`, `packages`). The list is `[]`, never missing, when nothing is found. Exit codes are as in text.
- **Shared item shapes:** a site is the same object in every command (`kind`, `pos`, `from`, `via`, `line`, plus the call or access fields), and `from` comes with `fromDecl`, the enclosing declared function, when `from` is a closure. A summary item carries its subject's `id` and `pos`, plus counts.
- **Trees** nest: each item has `children`. A repeat has `"seen": true` and no children, and a cycle `"recursive": true`.
- **Positions** are `path:line:col` strings, as printed. **References** to other nodes are IDs; only the subject is a whole node.
- **No versioning yet.** Until a release, the shapes can change. After one, fields are added, never renamed or removed.

## 7. Mermaid and DOT

These are for commands whose answer is a graph (section 3), to paste into an issue, PR or README.

- **What's drawn:** nodes are packages or functions, labeled by short name. Edges are imports or calls, and an edge is labeled only when it isn't a direct call (`via store.Getter`).
- **Same graph in both:** DOT has the same nodes and edges, with full IDs as node names.
- **Size:** Mermaid renders badly past about 100 nodes. The graph is still printed whole, and a note on stderr suggests `--depth` or a narrower path.

## 8. `grepo mcp`

One tool per command, with the command's flags as arguments. A result is the command's `--json` object. There's no working directory, so `base` is the module root and positions are relative to it. Text output isn't offered: agents get IDs and can ask again with them.

## 9. Changes before the next command

1. **Columns in positions:**
   - `graph.FormatPos` writes `path:line:col`, and `SplitPos` reads it, so the graph, `analyze` JSON, store shares and `callers` all change. Store keys hold grepo's binary, so old rows aren't reused.
   - Two calls on one line become two positions. vta-diff can then match them exactly, instead of treating a repeated call as matched (`vtadiff.Compare`).
   - Cost: about 390 position literals in tests change, most through helpers like `pat("35")`.
2. **`--format`**, shared by the query commands, with `--json` kept as its alias.
3. **`base`** in `callers --json`.
4. **The tree printer**, written with `callees` and then given to `callers` for `--depth`.

## 10. Open decisions

- **Columns now?** The test churn above is the cost. Recommended: yes, before more commands print positions.
- **Tree lines:** position first, with the indent in the message (section 4), or a drawn tree that editors can't read? Recommended: position first.
- **Order:** results by position (`callers`, `callees`, `state`), and ranked lists (`find`, `pkg` with no path) by use. Is that the right split?
- **Closures:** keep `$n` IDs in text (recommended), or show the enclosing function with a `[closure]` tag?
