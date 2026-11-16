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
	pkgs, err := checker.LoadPackages("rangetest")
	if err != nil {
		t.Fatal(err)
	}
	result := Result{}
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			t.Fatalf("errors loading %s: %v", pkg.ID, pkg.Errors)
		}
		result.Append(checker.CheckPackage(pkg))
	}
	result = result.Unique()

	if len(result.UncheckedErrors) != 3 {
		t.Fatalf("expected 3 errors, got %d: %v", len(result.UncheckedErrors), result.UncheckedErrors)
	}
	for _, e := range result.UncheckedErrors {
		if strings.Contains(e.Line, "seqs.Excluded()") {
			t.Errorf("excluded iterator was reported: %v", e)
		}
	}
}
