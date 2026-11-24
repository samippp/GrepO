package store

import (
	"net/http"

	"example.com/simple/model"
)

// Type refs: every place code names a type. Receivers, type params and
// built-in types aren't refs.
type Ref struct {
	Next *Ref
	model.User
	Req  *http.Request
	refs Refs
}

type Refs []Ref

type RefAlias = Ref

type Ordered interface{ ~int | ~string }

type Sorted[T Ordered] struct{ items []T }

type Namer interface {
	Name(r Ref) RefAlias
}

var defaultRef = Ref{}

func (r *Ref) refMethod() {}

func refsIn(x any, h http.Handler) *Ref {
	var a Ref
	_ = []*Ref{{Next: &a}}
	_ = RefAlias(a)
	_, _ = new(Ref), make([]Ref, 0)
	_, _ = x.(Ref)
	switch x.(type) {
	case *Ref, Refs:
	}
	_ = Sorted[int]{}
	_ = (*Ref).refMethod
	type local struct{ r Ref }
	var l local
	_ = func(r Ref) model.User { return l.r.User }
	return nil
}

func smallest[T Ordered](xs []T) T { return xs[0] }
