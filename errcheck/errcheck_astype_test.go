package errcheck

import (
	"fmt"
	"testing"
)

func TestErrorsAsType(t *testing.T) {
	const testGoMod = `module astypetest

go 1.26
`
	const testMain = `package main

import (
	"errors"
	stderrors "errors"
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

	// Both results assigned to blank
	_, _ = errors.AsType[MyCustomError](err)

	// Direct call statement
	errors.AsType[MyCustomError](err)

	// Renamed import of the standard library errors package
	stderrors.AsType[MyCustomError](err)
	_, _ = stderrors.AsType[MyCustomError](err)

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
		wantErrors(t, &checker, 0, "astypetest")
	})

	t.Run("ignored with default excluded symbols", func(t *testing.T) {
		var checker Checker
		checker.Exclusions.BlankAssignments = false
		checker.Exclusions.Symbols = append(checker.Exclusions.Symbols, DefaultExcludedSymbols...)
		wantErrors(t, &checker, 0, "astypetest")
	})
}

// TestErrorsAsTypeLookalike checks that only the standard library's
// errors.AsType is exempt, not a call that is merely spelled errors.AsType.
func TestErrorsAsTypeLookalike(t *testing.T) {
	const testGoMod = `module astypelookalike

go 1.26
`
	const testMain = `package main

import "errors"

type notErrors struct{}

func (notErrors) AsType(err error) (error, bool) { return err, false }

func testLookalike() {
	err := errors.New("failure")

	// A local variable named errors shadows the package.
	errors := notErrors{}
	errors.AsType(err)
	_, _ = errors.AsType(err)
}
`
	injectPackages(t, testGoMod, testMain)

	for _, defaults := range []bool{false, true} {
		t.Run(fmt.Sprintf("default excluded symbols %v", defaults), func(t *testing.T) {
			var checker Checker
			checker.Exclusions.BlankAssignments = false
			if defaults {
				checker.Exclusions.Symbols = append(checker.Exclusions.Symbols, DefaultExcludedSymbols...)
			}
			wantErrors(t, &checker, 2, "astypelookalike")
		})
	}
}
