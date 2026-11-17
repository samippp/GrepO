// Locals holding functions: f := s.Get; f(). A call through a local gets
// Funcs, every function assigned to it in its scope, closures' assignments
// included. A local's scope is within one package, so calls are resolved
// once the package is walked: an assignment can come after the call (loops).

package graph

import (
	"go/ast"
	"go/types"
	"maps"
	"slices"
)

type localCall struct {
	call  int        // index in b.calls
	local *types.Var // the local called
}

// local is the local var e is (params and results too), or nil.
func (b *callGraphBuilder) local(e ast.Expr) *types.Var {
	id, ok := ast.Unparen(e).(*ast.Ident)
	if !ok {
		return nil
	}
	v, ok := b.pkg.TypesInfo.ObjectOf(id).(*types.Var)
	if !ok || v.IsField() || v.Parent() == nil || v.Parent() == b.pkg.Types.Scope() {
		return nil
	}
	return v
}

// assignLocal records value as assigned to lhs, if lhs is a local and value
// is a function.
func (b *callGraphBuilder) assignLocal(lhs, value ast.Expr) {
	v := b.local(lhs)
	if v == nil {
		return
	}
	fn := b.funcValue(value)
	if fn == "" {
		return
	}
	if b.localFuncs[v] == nil {
		b.localFuncs[v] = map[string]bool{}
	}
	b.localFuncs[v][fn] = true
}

// callLocal notes that the call just added, b.calls' last, is made through
// fun, if fun is a local.
func (b *callGraphBuilder) callLocal(fun ast.Expr) {
	if v := b.local(fun); v != nil {
		b.localCalls = append(b.localCalls, localCall{call: len(b.calls) - 1, local: v})
	}
}

// resolveLocalCalls gives each call through a local its Funcs.
func (b *callGraphBuilder) resolveLocalCalls() {
	for _, lc := range b.localCalls {
		b.calls[lc.call].Funcs = slices.Sorted(maps.Keys(b.localFuncs[lc.local]))
	}
	b.localFuncs, b.localCalls = nil, nil
}
