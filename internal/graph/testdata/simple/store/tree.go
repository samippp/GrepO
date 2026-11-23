package store

// For callers --depth: walk recurses and reaches leaf twice.
func walk(n int) {
	if n > 0 {
		walk(n - 1)
	}
	visit()
	leaf()
}

func visit() { leaf() }

func leaf() {}
