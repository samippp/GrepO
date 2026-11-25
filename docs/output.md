# grepo output design

How every query command prints its answer: `callers`, `callees`, `find` and `state` (done), `pkg`, `entry`, `impact`, and `grepo mcp`. Commands get built to fit these rules, so later ones don't need their own conventions. `analyze` stays as it is: it dumps the whole graph, and isn't a query.

Status: agreed. Section 9 lists what changes before the next command, and section 10 the decisions made.

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
| `name` | what's at the position: for a `callers` site, the function it's in (`main.main`, `main.main$1` for a closure); for a `callees` site, the function it reaches (`(func value)` when nothing names it); for a declaration, what's declared |
| `source line` | the trimmed line at the position, read when printed; left out when the file can't be read |
| `[how]` | how the result relates to what was asked, if not plainly; one tag, at most a few words, its parts joined by commas (`[implements store.Getter, see above]`) |

A call site's position is the start of its call expression, and a declaration's is its name. This is the same rule the graph uses, so text, JSON and the graph agree.

Closures keep their IDs (`doActiveHealthCheck$1`), not their enclosing function's. The source line already gives the context, and the ID can be asked about. JSON adds the enclosing declared function (section 6).

## 3. Formats

`--format text|json|mermaid|dot`, with `--json` short for `--format json`. Every command has `text` and `json`. Only commands whose answer is a graph have `mermaid` and `dot`: `pkg`, `callers`/`callees` (at any depth, one level by default), and `impact`. Asking another command for them exits with 2.

## 4. Trees: `--depth`

`callers` and `callees` print one level by default: flat, as `callers` does now. `--depth n` follows n levels, and `--depth 0` goes all the way. Each line is still position-first. The tree is shown by indenting the name, two spaces per level, so quickfix and grep keep working:

```
$ grepo callees --depth 2 main.main
main.go:16:7: store.New: s := store.New()
main.go:17:7: (*store.Store).Get: u := s.Get(1)
store/store.go:24:2:   (*sync.Mutex).Lock: s.mu.Lock()
store/store.go:25:8:   (*sync.Mutex).Unlock: defer s.mu.Unlock()  [defer]
main.go:20:2: (store.Getter).Get: g.Get(2)  [interface]
store/cache.go:9:16:   (store.Cache).Get  [implements store.Getter]
store/store.go:23:17:   (*store.Store).Get  [implements store.Getter, see above]
main.go:23:2: main.greet: f(u.Name)  [through a local]
main.go:36:2:   fmt.Println: fmt.Println("hello", name)
main.go:25:2: main.main$1: func() {
main.go:26:3:   main.greet: greet("closure")  [see above]
main.go:32:2: fmt.Println: fmt.Println(u)
```

- **Interface calls** get their implementations as children. Each is located at its declaration and tagged `[implements I]`, and each expands in turn.
- **Repeats:** a function already expanded is printed again with `[see above]`, and isn't expanded twice, so `--depth 0` stays bounded (7,435 lines for `callees --depth 0 caddy.Load`). A site at the depth limit is marked too. A call back into the path above is `[recursive]`.
- **Leaves:** functions outside the repo aren't expanded, and calls to them show as leaves.
- **`--direct`** drops the interface, local and value edges at every level.
- **`callers`** expands a site's calling function: its children are the sites that can call that function. Package-level code is a leaf. A self-call is `[recursive]` at one level too, as in `callees`.

```
$ grepo callers --depth 0 leaf
store/tree.go:9:2: store.walk: leaf()
store/tree.go:6:3:   store.walk: walk(n - 1)  [recursive]
store/tree.go:12:16: store.visit: func visit() { leaf() }
store/tree.go:8:2:   store.walk: visit()  [see above]
```


A drawn tree (`├─`) was considered and rejected: it breaks rule 1, and quickfix can't read it.

## 5. Per command

`callers`, `callees`, `find` and `state` exist. The others are *sketches* of how they fit the rules. Each command's own design settles its details.

**`callers <func>`**: as now, plus columns. `--depth` makes it a tree (section 4).
```
main.go:17:7: main.main: u := s.Get(1)
main.go:20:2: main.main: g.Get(2)  [via store.Getter]
```

**`callees <func>`**: every site in a function that reaches another, by position: its calls, and the functions it passes or stores as values (`[passed to f]`, `[stored in field]`), as `callers` lists those the other way. An interface call is tagged `[interface]`. A call through a local gives a line per function assigned to it, tagged `[through a local]`. A call nothing resolves is `[dynamic]`, named by the field or var it goes through, or `(func value)`. `go` and `defer` join the tag (`[go]`, `[defer]`). `--depth` makes it a tree (section 4).

**`find <word>`**: the repo's declarations matching a word, most used first, ties by position. Each line is at the declaration (rule 3), its tag the kind and counts. `--kind` keeps some kinds.
```
$ grepo find get
store/store.go:12:6: store.Getter: type Getter interface {  [interface, 2 refs, 3 implementations]
store/store.go:13:2: (store.Getter).Get: Get(id int) model.User  [method, 1 caller]
store/store.go:23:17: (*store.Store).Get: func (s *Store) Get(id int) model.User {  [method, 1 caller, 1 more via an interface]
store/cache.go:9:16: (store.Cache).Get: func (c Cache) Get(id int) model.User { return model.User{ID: id} }  [method, no callers, 1 via an interface]
```
- **Funcs and methods** rank by callers: `callers`' sites, except those through an interface, which the tag counts as "more via interfaces". The two add up to `callers X | wc -l`.
- **Fields, vars and consts** rank by writes (assign, update, content, construct) plus reads (read, escape).
- **Types** rank by refs, the places code names them, with methods or, for an interface, implementations in the tag.
- Closures and types declared in functions aren't listed. JSON: `{base, word, results}`, each result with `kind`, `id`, `pos`, `line`, `uses` (what it's ranked by) and its kind's counts.

**`state <struct|field|var>`**: rule 3 in action. A struct gives a line per field, at the field, with its writes by kind and escapes. A field or var gives a line per site that writes it or hands out access to it, by position; `--reads` adds reads.
```
$ grepo state Holder
store/reads.go:23:2: store.Holder.N: N     int  [counter: 2 updates, 1 escape]
store/reads.go:24:2: store.Holder.Step: Step  int  [no writes found]
store/reads.go:26:2: store.Holder.Items: Items []int  [1 assign, 1 construct, 1 update, 4 escapes]
$ grepo state Store
store/store.go:17:2: store.Store.mu: mu    sync.Mutex  [no writes found, 2 escapes]
store/store.go:18:2: store.Store.users: users map[int]model.User  [set at construction: 1 construct]
$ grepo state --reads Store.users
store/store.go:21:35: store.New: func New() *Store { return &Store{users: map[int]model.User{}} }  [construct]
store/store.go:26:9: (*store.Store).Get: return s.users[id]  [read]
```
- **Roles** only where the writes settle one: "no writes found", "set at construction" (construct writes only), "counter" (a number, update writes only). Escapes are always counted, so a field with none of its own writes doesn't read as unchanged. "found" because writes by reflection (`encoding/json` filling a config struct) aren't seen: on caddy, 164 of 1,537 fields have no writes.
- **Names** as `callers` takes them; an alias of a struct is the struct. Repo types, fields and vars only, for now.
- JSON: a struct gives `{base, type, fields}`, each field with `role`, `writes` by kind and `escapes`; a field or var gives `{base, field | var, sites}`, sites in the shared shape with `kind` write, escape or read and `access` the access kind.

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

```
$ grepo callees --depth 2 --format mermaid main
flowchart LR
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
```

- **What's drawn:** nodes are packages or functions, labeled by short name. Edges are imports or calls, and an edge is labeled only when it isn't a direct call (`through a local`, `passed to f`, `stored in X`, `dynamic`).
- **Call trees** (`callers`, `callees`): a node per function, and an edge per caller, callee and label, pointing from caller to callee in both commands. Several call sites become one edge. A repeat or a recursive call is an edge to the node already drawn, so there's no `[see above]`.
- **Interface calls** go to the interface method, drawn as a hexagon, with dotted edges from it to its implementations. `callers` draws the method too, from the call's callee. Collapsing it into `via I` edges from each caller to each implementation was weighed and rejected:
  - edges grow as callers times implementations: 7,533 edges against 5,102 for `callees --depth 0 caddy.Load`;
  - a call with no implementation in the repo (`io.Writer`), or at the depth limit, would still need the method's node.
- **Other nodes:** the function asked about has a bold border, and functions outside the repo a dashed one. Package-level code is `store (package level)`. A call nothing resolves goes to a node named by the field or var it goes through, or to a `(func value)` node per caller.
- **Same graph in both:** DOT has the same nodes and edges, with full IDs as node names.
- **Size:** Mermaid renders badly past about 100 nodes. The graph is still printed whole, and a note on stderr suggests `--depth` or a narrower path.

## 8. `grepo mcp`

One tool per command, with the command's flags as arguments. A result is the command's `--json` object. There's no working directory, so `base` is the module root and positions are relative to it. Text output isn't offered: agents get IDs and can ask again with them.

## 9. Changes before the next command

1. **Columns in positions** (done):
   - `graph.FormatPos` writes `path:line:col`, and `SplitPos` reads it, so the graph, `analyze` JSON, store shares and `callers` all change. Store keys hold grepo's binary, so old rows aren't reused.
   - Two calls on one line become two positions. vta-diff can then match them exactly, instead of treating a repeated call as matched (`vtadiff.Compare`).
   - Cost: about 390 position literals in tests change, most through helpers like `pat("35")`.
2. **`--format`** (done), shared by the query commands (`addFormatFlags`), with `--json` kept as its alias.
3. **`base`** (done) in `callers --json`, with `fromDecl` on closure sites (`Index.Decl`).
4. **The tree printer** (done): `query.CalleeTree` and `query.CallerTree` share one builder, keyed by a site's `Target` and `From`.

## 10. Decisions

- **Columns now:** yes, before more commands print positions (section 9, item 1).
- **Tree lines:** position first, with the indent in the message (section 4).
- **Interface calls in diagrams:** the interface method is a node of its own (section 7).
- **Order:** results by position (`callers`, `callees`, `state`); ranked lists by use (`find`, `pkg` with no path), most used first. On caddy, `find Load` puts `(caddy.Context).LoadModule` (60 callers) first, where by position it would come fifth, behind a `LoadConfig` nothing calls. `| sort` gives position order back.
- **`state` roles:** counts always, a role only where the counts settle it (above). "config" and "live state", as first sketched, were guesses.
- **`find` matching:** a word without a dot matches a declaration's own name, and one with a dot its short name with `(`, `)` and `*` removed (`Context.Load`, `caddytls.`). Both are case-insensitive substrings. Matching plain words against qualified names too would list everything in package `loader` for `find load`.
- **Closures and inits:** text keeps their IDs (`replaceLocalAdminServer$1$1`, `main.init@main.go#1`), not the enclosing function with a tag. The ID can be passed back as a name, and `$1$1` shows the code runs later (a goroutine or callback), not when its parent runs. JSON adds `fromDecl` (section 6).
