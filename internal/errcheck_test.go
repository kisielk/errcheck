package errcheck

import (
	"go/build"
	"go/parser"
	"go/token"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

const testPackage = "github.com/kisielk/errcheck/testdata"

var (
	unchecked map[marker]bool
	blank     map[marker]bool
)

type marker struct {
	file string
	line int
}

func init() {
	unchecked = make(map[marker]bool)
	blank = make(map[marker]bool)

	pkg, err := build.Import(testPackage, "", 0)
	if err != nil {
		panic("failed to import test package")
	}
	fset := token.NewFileSet()
	astPkg, err := parser.ParseDir(fset, pkg.Dir, nil, parser.ParseComments)
	if err != nil {
		panic("failed to parse test package")
	}

	for _, file := range astPkg["main"].Files {
		for _, comment := range file.Comments {
			text := comment.Text()
			pos := fset.Position(comment.Pos())
			switch text {
			case "UNCHECKED\n":
				unchecked[marker{pos.Filename, pos.Line}] = true
			case "BLANK\n":
				blank[marker{pos.Filename, pos.Line}] = true
			}
		}
	}
}

// TestUnchecked runs a test against the example files and ensures all unchecked errors are caught.
func TestUnchecked(t *testing.T) {
	err := CheckPackages([]string{testPackage}, make(map[string]*regexp.Regexp), make([]string, 0), false, true)
	uerr, ok := err.(UncheckedErrors)
	if !assert.True(t, ok, "error is an UncheckedErrors error") {
		t.Fatal("wrong error type returned")
	}

	numErrors := len(unchecked)
	if !assert.Equal(t, len(uerr.Errors), numErrors, "got %d errors", len(uerr.Errors)) {
		for i, err := range uerr.Errors {
			t.Errorf("%d: %v", i, err)
		}
		return
	}

	for i, err := range uerr.Errors {
		uerr, ok := err.(uncheckedError)
		if !assert.True(t, ok, "error %d is an UncheckedError error", i) {
			t.Errorf("%d: not an uncheckedError, got %v", i, err)
			continue
		}
		m := marker{uerr.pos.Filename, uerr.pos.Line}
		assert.True(t, unchecked[m], "expected error at %v", m)
	}
}

// TestBlank is like TestUnchecked but also ensures assignments to the blank identifier are caught.
func TestBlank(t *testing.T) {
	err := CheckPackages([]string{testPackage}, make(map[string]*regexp.Regexp), make([]string, 0), true, true)
	uerr, ok := err.(UncheckedErrors)
	if !assert.True(t, ok, "error is an UncheckedErrors error") {
		t.Fatal("wrong error type returned")
	}

	numErrors := len(unchecked) + len(blank)
	if !assert.Equal(t, len(uerr.Errors), numErrors, "got %d errors", len(uerr.Errors)) {
		for i, err := range uerr.Errors {
			t.Errorf("%d: %v", i, err)
		}
		return
	}

	for i, err := range uerr.Errors {
		uerr, ok := err.(uncheckedError)
		if !assert.True(t, ok, "error %d is an UncheckedError error", i) {
			t.Errorf("%d: not an uncheckedError, got %v", i, err)
			continue
		}
		m := marker{uerr.pos.Filename, uerr.pos.Line}
		assert.True(t, unchecked[m] || blank[m], "expected error at %v", m)
	}
}
