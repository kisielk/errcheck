package errcheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	for range seqs.Checked() {
	}
	for i, _ := range seqs.Checked() {
		_ = i
	}

	// Reported: there is no call to look up in the exclusion list.
	seq := seqs.Excluded()
	for range seq {
	}
}
`
	const testSeqs = `package seqs

import "iter"

func Excluded() iter.Seq2[int, error] { return func(func(int, error) bool) {} }

func Checked() iter.Seq2[int, error] { return func(func(int, error) bool) {} }
`
	dir := injectPackages(t, testGoMod, testMain)
	if err := os.Mkdir(filepath.Join(dir, "seqs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "seqs", "seqs.go"), []byte(testSeqs), 0644); err != nil {
		t.Fatal(err)
	}

	var checker Checker
	checker.Exclusions.Symbols = []string{"rangetest/seqs.Excluded"}
	errs := wantErrors(t, &checker, 3, "rangetest")
	for _, e := range errs {
		if strings.Contains(e.Line, "seqs.Excluded()") {
			t.Errorf("excluded iterator was reported: %v", e)
		}
	}
}
