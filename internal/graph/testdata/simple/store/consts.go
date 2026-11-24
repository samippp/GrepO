package store

import (
	"time"

	"example.com/simple/model"
)

// A repo const from another package is read, like one from this package.
func tagLimit() int { return model.MaxTag }

// So is one in a generic type's constraint (#69).
type bounded[T ~[model.MaxTag]byte] struct{ v T }

// A package only a type ref names is a dependency too.
var timeout time.Duration
