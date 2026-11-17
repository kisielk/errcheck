package errcheck

import "testing"

// TestRangeStmtExclusions checks that ranging over a call to an excluded
// iterator function is not reported, whether its error is left out or
// assigned to a blank identifier, while other iterators still are.
func TestRangeStmtExclusions(t *testing.T) {
	const testGoMod = `module rangetest

go 1.26
`
	const testMain = `package main

import "rangetest/seqs"

func main() {
	// Excluded by name.
	for range seqs.Excluded() {
	}
	for i, _ := range seqs.Excluded() {
		_ = i
	}

	// Reported.
	for range seqs.Checked() { // UNCHECKED
	}
	for i, _ := range seqs.Checked() { // BLANK
		_ = i
	}

	// Reported: there is no call to look up in the exclusion list.
	seq := seqs.Excluded()
	for range seq { // UNCHECKED
	}
}
`
	const testSeqs = `package seqs

import "iter"

func Excluded() iter.Seq2[int, error] { return func(func(int, error) bool) {} }

func Checked() iter.Seq2[int, error] { return func(func(int, error) bool) {} }
`
	useTempModule(t, map[string]string{
		"go.mod":       testGoMod,
		"main.go":      testMain,
		"seqs/seqs.go": testSeqs,
	})

	var checker Checker
	checker.Exclusions.Symbols = []string{"rangetest/seqs.Excluded"}
	wantMarked(t, &checker, "rangetest")
}
