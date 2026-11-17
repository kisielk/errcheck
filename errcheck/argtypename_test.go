package errcheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	fmt.Fprintf(os.Stdout, "x")
	fmt.Fprintln(os.Stderr, "x")

	// Parenthesized arguments are matched the same way.
	fmt.Fprintf((stdos.Stdout), "x")
	fmt.Fprintf((os.Stdout), "x")
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
	dir := injectPackages(t, testGoMod, testMain)
	if err := os.Mkdir(filepath.Join(dir, "os"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "os", "os.go"), []byte(lookalikeOs), 0644); err != nil {
		t.Fatal(err)
	}

	var checker Checker
	checker.Exclusions.Symbols = append(checker.Exclusions.Symbols, DefaultExcludedSymbols...)
	errs := wantErrors(t, &checker, 3, "argnametest")
	for _, e := range errs {
		if strings.Contains(e.Line, "stdos.") {
			t.Errorf("standard library os call was not excluded: %v", e)
		}
	}
}
