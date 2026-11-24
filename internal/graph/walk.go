// The walker: recursive descent over function bodies and package-level values.
// A subexpression is a read only if it reaches expr; see docs/architecture.md.

package graph

import (
	"go/ast"
	"go/token"
	"go/types"
)

// walk records the calls, accesses, type refs and local types under n, a
// statement or declaration in id's body (or a package-level value, with the
// package as id).
func (b *callGraphBuilder) walk(id string, n ast.Node) {
	switch n := n.(type) {
	case nil:
		return
	case ast.Expr:
		b.expr(id, n)
		return
	case *ast.GenDecl:
		for _, spec := range n.Specs {
			switch spec := spec.(type) {
			case *ast.TypeSpec:
				b.assignLocalTypeID(id, spec)
				b.handleTypeSpec(spec)
				b.typeSpec(id, spec)
			case *ast.ValueSpec:
				b.expr(id, spec.Type)
				for i, value := range spec.Values {
					if len(spec.Values) == len(spec.Names) {
						b.assignLocal(spec.Names[i], value) // var f = s.Get
					}
					b.handed(id, value)
				}
			}
		}
		return
	case *ast.AssignStmt:
		b.addAssign(id, n)
		return
	case *ast.IncDecStmt:
		b.addWrite(id, n.X, UpdateAccess, "")
		return
	case *ast.RangeStmt:
		if n.Tok == token.ASSIGN { // for x = range: := declares locals instead
			b.addWrite(id, n.Key, AssignAccess, "")
			b.addWrite(id, n.Value, AssignAccess, "")
		}
		b.expr(id, n.X)
		b.walk(id, n.Body)
		return
	case *ast.ReturnStmt:
		for _, result := range n.Results {
			b.handed(id, result)
		}
		return
	case *ast.GoStmt:
		b.stmt[n.Call] = "go"
		b.expr(id, n.Call)
		return
	case *ast.DeferStmt:
		b.stmt[n.Call] = "defer"
		b.expr(id, n.Call)
		return
	case *ast.SendStmt:
		b.expr(id, n.Chan)
		b.handed(id, n.Value)
		return
	}
	b.children(id, n) // if, for, switch, blocks, …: nothing changes context
}

// expr records what's under e, an expression evaluated as a value.
func (b *callGraphBuilder) expr(id string, e ast.Expr) {
	if e == nil {
		return
	}
	if b.suppressed[e] {
		delete(b.suppressed, e)
		return // an update's matching expression: idents and selectors only, nothing else inside
	}
	switch e := e.(type) {
	case *ast.Ident:
		b.name(id, b.pkg.TypesInfo.Uses[e], e.Pos())
		return
	case *ast.SelectorExpr:
		b.addSelector(id, e)
		return
	case *ast.UnaryExpr:
		if e.Op == token.AND {
			b.addAddress(id, e.X)
			return
		}
	case *ast.CallExpr:
		written := b.addBuiltinWrite(id, e)
		b.addCall(id, e)
		b.expr(id, e.Fun) // a func field or var is read; receivers by addSelector
		args := e.Args
		if written {
			args = args[1:]
		}
		isCall := b.isCall(e)
		for _, arg := range args {
			if isCall {
				b.handed(id, arg)
			} else {
				b.expr(id, arg)
			}
		}
		return
	case *ast.CompositeLit:
		b.addConstruct(id, e)
		b.expr(id, e.Type) // consts in it: [Max]byte{}
		_, st := b.literalStruct(e)
		for _, elt := range e.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				if st == nil {
					b.handed(id, kv.Key) // map keys; struct keys are field names
				}
				b.handed(id, kv.Value)
			} else {
				b.handed(id, elt)
			}
		}
		return
	case *ast.FuncLit:
		cid := b.addClosure(e) // what's inside belongs to the closure
		b.expr(cid, e.Type)
		b.walk(cid, e.Body)
		return
	}
	b.children(id, e) // binary, index, slice, star, paren, type assertion, …
}

// name records what a name in an expression is: a value read, or a type ref.
func (b *callGraphBuilder) name(id string, obj types.Object, pos token.Pos) {
	if tn, ok := obj.(*types.TypeName); ok {
		b.addTypeRef(id, tn, pos)
		return
	}
	b.addValueRead(id, obj, pos)
}

// typeSpec walks a type declaration's type params and type, for the consts
// and types they name: type T[P ~[Max]byte] struct{ next *T[P] }.
func (b *callGraphBuilder) typeSpec(id string, spec *ast.TypeSpec) {
	if spec.TypeParams != nil { // a nil *FieldList as ast.Node isn't == nil
		b.walk(id, spec.TypeParams)
	}
	b.expr(id, spec.Type)
}

// children walks n's direct children.
func (b *callGraphBuilder) children(id string, n ast.Node) {
	ast.Inspect(n, func(c ast.Node) bool {
		if c == n {
			return true
		}
		b.walk(id, c)
		return false // walk handles c's own children
	})
}
