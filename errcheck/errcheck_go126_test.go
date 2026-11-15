//go:build go1.26

package errcheck

import (
	"testing"
)

func TestErrorsAsType(t *testing.T) {
	const testGoMod = `module astypetest

go 1.26
`
	const testMain = `package main

import (
	"errors"
	"io/fs"
	"os"
)

type MyCustomError struct {
	msg string
}

func (e MyCustomError) Error() string {
	return e.msg
}

func testErrorsAsType() {
	var err error = MyCustomError{msg: "failure"}

	// Blank assignment with ok check
	if _, ok := errors.AsType[MyCustomError](err); !ok {
		return
	}

	// Single blank assignment
	_ = errors.AsType[MyCustomError](err)

	// Direct call statement
	errors.AsType[MyCustomError](err)

	// Standard library pointer error type
	_, openErr := os.Open("non-existent")
	if _, ok := errors.AsType[*fs.PathError](openErr); ok {
		return
	}
}
`
	injectPackages(t, testGoMod, testMain)

	t.Run("ignored with blank assignments enabled", func(t *testing.T) {
		var checker Checker
		checker.Exclusions.BlankAssignments = false
		pkgs, err := checker.LoadPackages("astypetest")
		if err != nil {
			t.Fatal(err)
		}
		result := Result{}
		for _, pkg := range pkgs {
			result.Append(checker.CheckPackage(pkg))
		}
		result = result.Unique()
		if len(result.UncheckedErrors) != 0 {
			t.Errorf("expected 0 errors, got %d: %v", len(result.UncheckedErrors), result.UncheckedErrors)
		}
	})

	t.Run("ignored with default excluded symbols", func(t *testing.T) {
		var checker Checker
		checker.Exclusions.BlankAssignments = false
		checker.Exclusions.Symbols = append(checker.Exclusions.Symbols, DefaultExcludedSymbols...)
		pkgs, err := checker.LoadPackages("astypetest")
		if err != nil {
			t.Fatal(err)
		}
		result := Result{}
		for _, pkg := range pkgs {
			result.Append(checker.CheckPackage(pkg))
		}
		result = result.Unique()
		if len(result.UncheckedErrors) != 0 {
			t.Errorf("expected 0 errors, got %d: %v", len(result.UncheckedErrors), result.UncheckedErrors)
		}
	})
}
