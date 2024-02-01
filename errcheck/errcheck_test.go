package errcheck

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

const testPackage = "github.com/kisielk/errcheck/testdata"

var (
	uncheckedMarkers map[marker]bool
	blankMarkers     map[marker]bool
	assertMarkers    map[marker]bool
)

type marker struct {
	file string
	line int
}

func newMarker(e UncheckedError) marker {
	return marker{e.Pos.Filename, e.Pos.Line}
}

func (m marker) String() string {
	return fmt.Sprintf("%s:%d", m.file, m.line)
}

func init() {
	uncheckedMarkers = make(map[marker]bool)
	blankMarkers = make(map[marker]bool)
	assertMarkers = make(map[marker]bool)

	cfg := &packages.Config{
		Mode:  packages.NeedSyntax | packages.NeedTypes,
		Tests: true,
	}
	pkgs, err := packages.Load(cfg, testPackage)
	if err != nil {
		panic(fmt.Errorf("failed to import test package: %v", err))
	}
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			for _, comment := range file.Comments {
				text := comment.Text()
				pos := pkg.Fset.Position(comment.Pos())
				m := marker{pos.Filename, pos.Line}
				switch text {
				case "UNCHECKED\n":
					uncheckedMarkers[m] = true
				case "BLANK\n":
					blankMarkers[m] = true
				case "ASSERT\n":
					assertMarkers[m] = true
				}
			}
		}
	}
}

type flags uint

const (
	CheckAsserts flags = 1 << iota
	CheckBlank
)

// TestUnchecked runs a test against the example files and ensures all unchecked errors are caught.
func TestUnchecked(t *testing.T) {
	test(t, 0)
}

// TestBlank is like TestUnchecked but also ensures assignments to the blank identifier are caught.
func TestBlank(t *testing.T) {
	test(t, CheckBlank)
}

func TestAll(t *testing.T) {
	// TODO: CheckAsserts should work independently of CheckBlank
	test(t, CheckAsserts|CheckBlank)
}

func TestBuildTags(t *testing.T) {
	const (
		// uses "custom1" build tag and contains 1 unchecked error
		testBuildCustom1Tag = `
` + `// +build custom1

package custom

import "fmt"

func Print1() {
	// returns an error that is not checked
	fmt.Fprintln(nil)
}`
		// uses "custom2" build tag and contains 1 unchecked error
		testBuildCustom2Tag = `
` + `// +build custom2

package custom

import "fmt"

func Print2() {
	// returns an error that is not checked
	fmt.Fprintln(nil)
}`
		// included so that package is not empty when built without specifying tags
		testDoc = `
// Package custom contains code for testing build tags.
package custom
`
	)

	tmpGopath := t.TempDir()
	testBuildTagsDir := path.Join(tmpGopath, "src", "github.com/testbuildtags")
	if err := os.MkdirAll(testBuildTagsDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	if err := os.WriteFile(path.Join(testBuildTagsDir, "go.mod"), []byte("module github.com/testbuildtags"), 0644); err != nil {
		t.Fatalf("Failed to write testbuildtags go.mod: %v", err)
	}
	if err := os.WriteFile(path.Join(testBuildTagsDir, "custom1.go"), []byte(testBuildCustom1Tag), 0644); err != nil {
		t.Fatalf("Failed to write testbuildtags custom1: %v", err)
	}
	if err := os.WriteFile(path.Join(testBuildTagsDir, "custom2.go"), []byte(testBuildCustom2Tag), 0644); err != nil {
		t.Fatalf("Failed to write testbuildtags custom2: %v", err)
	}
	if err := os.WriteFile(path.Join(testBuildTagsDir, "doc.go"), []byte(testDoc), 0644); err != nil {
		t.Fatalf("Failed to write testbuildtags doc: %v", err)
	}

	cases := []struct {
		tags            []string
		numExpectedErrs int
	}{
		// with no tags specified, main is ignored and there are no errors
		{
			tags:            nil,
			numExpectedErrs: 0,
		},
		// specifying "custom1" tag includes file with 1 error
		{
			tags:            []string{"custom1"},
			numExpectedErrs: 1,
		},
		// specifying "custom1" and "custom2" tags includes 2 files with 1 error each
		{
			tags:            []string{"custom1", "custom2"},
			numExpectedErrs: 2,
		},
	}

	for _, test := range cases {
		testName := strings.Join(test.tags, ",")
		t.Run(testName, func(t *testing.T) {
			var checker Checker
			checker.Tags = test.tags

			loadPackages = func(cfg *packages.Config, paths ...string) ([]*packages.Package, error) {
				cfg.Env = append(os.Environ(),
					"GOPATH="+tmpGopath)
				cfg.Dir = testBuildTagsDir
				pkgs, err := packages.Load(cfg, paths...)
				return pkgs, err
			}
			packages, err := checker.LoadPackages("github.com/testbuildtags")
			if err != nil {
				t.Fatal(err)
			}

			uerr := &Result{}
			for _, pkg := range packages {
				uerr.Append(checker.CheckPackage(pkg))
			}
			*uerr = uerr.Unique()
			if test.numExpectedErrs == 0 {
				if len(uerr.UncheckedErrors) != 0 {
					t.Errorf("expected no errors, but got: %v", uerr)
				}
				return
			}

			if test.numExpectedErrs != len(uerr.UncheckedErrors) {
				t.Errorf("expected: %d errors\nactual:   %d errors", test.numExpectedErrs, len(uerr.UncheckedErrors))
			}
		})
	}
}

func TestWhitelist(t *testing.T) {

}

func TestIgnore(t *testing.T) {
	const testVendorGoMod = `module github.com/testvendor

require github.com/testlog v0.0.0
`
	const testVendorMain = `
	package main

	import "github.com/testlog"

	func main() {
		// returns an error that is not checked
		testlog.Info()
	}`
	const testLog = `
	package testlog

	func Info() error {
		return nil
	}`

	// copy testvendor directory into directory for test
	tmpGopath := t.TempDir()
	testVendorDir := path.Join(tmpGopath, "src", "github.com/testvendor")
	if err := os.MkdirAll(testVendorDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	if err := os.WriteFile(path.Join(testVendorDir, "go.mod"), []byte(testVendorGoMod), 0755); err != nil {
		t.Fatalf("Failed to write testvendor go.mod: %v", err)
	}
	if err := os.WriteFile(path.Join(testVendorDir, "main.go"), []byte(testVendorMain), 0755); err != nil {
		t.Fatalf("Failed to write testvendor main: %v", err)
	}
	if err := os.MkdirAll(path.Join(testVendorDir, "vendor/github.com/testlog"), 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(path.Join(testVendorDir, "vendor/github.com/testlog/testlog.go"), []byte(testLog), 0755); err != nil {
		t.Fatalf("Failed to write testlog: %v", err)
	}

	cases := []struct {
		ignore          map[string]*regexp.Regexp
		numExpectedErrs int
	}{
		// basic case has one error
		{
			ignore:          nil,
			numExpectedErrs: 1,
		},
		// ignoring vendored import works
		{
			ignore: map[string]*regexp.Regexp{
				path.Join("github.com/testvendor/vendor/github.com/testlog"): regexp.MustCompile("Info"),
			},
		},
		// non-vendored path ignores vendored import
		{
			ignore: map[string]*regexp.Regexp{
				"github.com/testlog": regexp.MustCompile("Info"),
			},
		},
	}

	for i, test := range cases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			var checker Checker
			checker.Exclusions.SymbolRegexpsByPackage = test.ignore
			loadPackages = func(cfg *packages.Config, paths ...string) ([]*packages.Package, error) {
				cfg.Env = append(os.Environ(),
					"GOPATH="+tmpGopath,
					"GOFLAGS=-mod=vendor")
				cfg.Dir = testVendorDir
				pkgs, err := packages.Load(cfg, paths...)
				return pkgs, err
			}
			packages, err := checker.LoadPackages("github.com/testvendor")
			if err != nil {
				t.Fatal(err)
			}
			uerr := &Result{}
			for _, pkg := range packages {
				uerr.Append(checker.CheckPackage(pkg))
			}
			*uerr = uerr.Unique()

			if test.numExpectedErrs == 0 {
				if len(uerr.UncheckedErrors) != 0 {
					t.Errorf("expected no errors, but got: %v", uerr)
				}
				return
			}

			if test.numExpectedErrs != len(uerr.UncheckedErrors) {
				t.Errorf("expected: %d errors\nactual:   %d errors", test.numExpectedErrs, len(uerr.UncheckedErrors))
			}
		})
	}
}

func TestWithoutGeneratedCode(t *testing.T) {
	const testVendorGoMod = `module github.com/testvendor

require github.com/testlog v0.0.0
`
	const testVendorMain = `
	// Code generated by protoc-gen-go. DO NOT EDIT.
	package main

	import "github.com/testlog"

	func main() {
		// returns an error that is not checked
		testlog.Info()
	}`
	const testLog = `
	package testlog

	func Info() error {
		return nil
	}`

	// copy testvendor directory into directory for test
	tmpGopath := t.TempDir()
	testVendorDir := path.Join(tmpGopath, "src", "github.com/testvendor")
	if err := os.MkdirAll(testVendorDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	if err := os.WriteFile(path.Join(testVendorDir, "go.mod"), []byte(testVendorGoMod), 0755); err != nil {
		t.Fatalf("Failed to write testvendor go.mod: %v", err)
	}
	if err := os.WriteFile(path.Join(testVendorDir, "main.go"), []byte(testVendorMain), 0755); err != nil {
		t.Fatalf("Failed to write testvendor main: %v", err)
	}
	if err := os.MkdirAll(path.Join(testVendorDir, "vendor/github.com/testlog"), 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(path.Join(testVendorDir, "vendor/github.com/testlog/testlog.go"), []byte(testLog), 0755); err != nil {
		t.Fatalf("Failed to write testlog: %v", err)
	}

	cases := []struct {
		withoutGeneratedCode bool
		numExpectedErrs      int
		withModVendor        bool
	}{
		// basic case has one error
		{
			withoutGeneratedCode: false,
			numExpectedErrs:      1,
		},
		// ignoring generated code works
		{
			withoutGeneratedCode: true,
			numExpectedErrs:      0,
		},
		// using checker.Mod="vendor"
		{
			withoutGeneratedCode: false,
			numExpectedErrs:      1,
			withModVendor:        true,
		},
	}

	for i, test := range cases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			var checker Checker
			checker.Exclusions.GeneratedFiles = test.withoutGeneratedCode
			if test.withModVendor {
				if os.Getenv("GO111MODULE") == "off" {
					t.Skip("-mod=vendor doesn't work if modules are disabled")
				}
				checker.Mod = "vendor"
			}
			loadPackages = func(cfg *packages.Config, paths ...string) ([]*packages.Package, error) {
				cfg.Env = append(os.Environ(),
					"GOPATH="+tmpGopath)

				if !test.withModVendor {
					cfg.Env = append(cfg.Env,
						"GOFLAGS=-mod=vendor")
				}
				cfg.Dir = testVendorDir
				pkgs, err := packages.Load(cfg, paths...)
				return pkgs, err
			}
			packages, err := checker.LoadPackages("github.com/testvendor")
			if err != nil {
				t.Fatal(err)
			}
			uerr := Result{}
			for _, pkg := range packages {
				uerr.Append(checker.CheckPackage(pkg))
			}
			uerr = uerr.Unique()

			if test.numExpectedErrs == 0 {
				if len(uerr.UncheckedErrors) != 0 {
					t.Errorf("expected no errors, but got: %v", uerr)
				}
				return
			}

			if test.numExpectedErrs != len(uerr.UncheckedErrors) {
				t.Errorf("expected: %d errors\nactual:   %d errors", test.numExpectedErrs, len(uerr.UncheckedErrors))
			}
		})
	}
}

func test(t *testing.T, f flags) {
	var (
		asserts bool = f&CheckAsserts != 0
		blank   bool = f&CheckBlank != 0
	)
	var checker Checker
	checker.Exclusions.TypeAssertions = !asserts
	checker.Exclusions.BlankAssignments = !blank
	checker.Exclusions.Symbols = append(checker.Exclusions.Symbols, DefaultExcludedSymbols...)
	checker.Exclusions.Symbols = append(checker.Exclusions.Symbols,
		fmt.Sprintf("(%s.ErrorMakerInterface).MakeNilError", testPackage),
	)
	packages, err := checker.LoadPackages(testPackage)
	if err != nil {
		t.Fatal(err)
	}
	uerr := Result{}
	numErrors := len(uncheckedMarkers)
	if blank {
		numErrors += len(blankMarkers)
	}
	if asserts {
		numErrors += len(assertMarkers)
	}

	for _, pkg := range packages {
		err := checker.CheckPackage(pkg)
		uerr.Append(err)
	}

	uerr = uerr.Unique()

	if len(uerr.UncheckedErrors) != numErrors {
		t.Errorf("got %d errors, want %d", len(uerr.UncheckedErrors), numErrors)
	unchecked_loop:
		for k := range uncheckedMarkers {
			for _, e := range uerr.UncheckedErrors {
				if newMarker(e) == k {
					continue unchecked_loop
				}
			}
			t.Errorf("Expected unchecked at %s", k)
		}
		if blank {
		blank_loop:
			for k := range blankMarkers {
				for _, e := range uerr.UncheckedErrors {
					if newMarker(e) == k {
						continue blank_loop
					}
				}
				t.Errorf("Expected blank at %s", k)
			}
		}
		if asserts {
		assert_loop:
			for k := range assertMarkers {
				for _, e := range uerr.UncheckedErrors {
					if newMarker(e) == k {
						continue assert_loop
					}
				}
				t.Errorf("Expected assert at %s", k)
			}
		}
	}

	for i, err := range uerr.UncheckedErrors {
		m := marker{err.Pos.Filename, err.Pos.Line}
		if !uncheckedMarkers[m] && !blankMarkers[m] && !assertMarkers[m] {
			t.Errorf("%d: unexpected error: %v", i, err)
		}
		if err.SelectorName != "" && !strings.Contains(err.Line, err.SelectorName) {
			t.Errorf("the line '%s' must contain the selector '%s'", err.Line, err.SelectorName)
		}
	}
}
