package errcheck

import "testing"

// TestExcludeCallsByName checks that exclusions apply to functions called by
// name alone, in their own package or after a dot import, and not only to
// calls through a selector like pkg.F(). See issue #251.
func TestExcludeCallsByName(t *testing.T) {
	const testGoMod = `module testerrcheck

go 1.26
`
	// firstGo declares the functions, and calls them by name in their own
	// package. FirstFunc and Generic are excluded; OtherFunc is not.
	const firstGo = `package first

func FirstFunc() error { return nil }

func Generic[T any]() error { return nil }

func OtherFunc() error { return nil }

func ThirdFunc() error {
	FirstFunc()
	Generic[int]()
	OtherFunc() // UNCHECKED
	return nil
}
`
	// secondGo calls the functions through a selector, which was already
	// excluded before #251 was fixed.
	const secondGo = `package second

import "testerrcheck/first"

func SecondFunc() error {
	first.FirstFunc()
	first.OtherFunc() // UNCHECKED
	return nil
}
`
	// thirdGo calls the functions by name after a dot import.
	const thirdGo = `package third

import . "testerrcheck/first"

func DotImport() error {
	FirstFunc()
	OtherFunc() // UNCHECKED
	return nil
}
`
	useTempModule(t, map[string]string{
		"go.mod":           testGoMod,
		"first/first.go":   firstGo,
		"second/second.go": secondGo,
		"third/third.go":   thirdGo,
	})

	var checker Checker
	checker.Exclusions.Symbols = []string{
		"testerrcheck/first.FirstFunc",
		"testerrcheck/first.Generic",
	}
	wantMarked(t, &checker, "testerrcheck/...")
}
