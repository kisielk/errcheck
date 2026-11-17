package errcheck

import "testing"

// TestArgTypeNameOsLookalike checks that the "fmt.Fprintf(os.Stdout)"-style
// default exclusions match only the standard library's os.Stdout and
// os.Stderr, not variables of the same name in another package named os.
func TestArgTypeNameOsLookalike(t *testing.T) {
	const testGoMod = `module argnametest

go 1.26
`
	const testMain = `package main

import (
	"fmt"
	stdos "os"

	"argnametest/os"
)

func main() {
	// The standard library's os, under another name: excluded.
	fmt.Fprintf(stdos.Stdout, "x")
	fmt.Fprintln(stdos.Stderr, "x")

	// A different package that is also named os: not excluded.
	fmt.Fprintf(os.Stdout, "x") // UNCHECKED
	fmt.Fprintln(os.Stderr, "x") // UNCHECKED

	// Parenthesized arguments are matched the same way.
	fmt.Fprintf((stdos.Stdout), "x")
	fmt.Fprintf((os.Stdout), "x") // UNCHECKED
}
`
	const lookalikeOs = `package os

type writer struct{}

func (writer) Write(p []byte) (int, error) { return len(p), nil }

var (
	Stdout writer
	Stderr writer
)
`
	useTempModule(t, map[string]string{
		"go.mod":   testGoMod,
		"main.go":  testMain,
		"os/os.go": lookalikeOs,
	})

	var checker Checker
	checker.Exclusions.Symbols = append(checker.Exclusions.Symbols, DefaultExcludedSymbols...)
	wantMarked(t, &checker, "argnametest")
}
