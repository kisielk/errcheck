package errcheck

import (
	"fmt"
	"go/token"
	"os"
	"path"
	"regexp"
	"slices"
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

// injectPackages writes goMod and mainGo to a temporary directory, and overrides
// loadPackages to load packages from that directory for the duration of the test.
// It returns the path to the temporary directory.
func injectPackages(t *testing.T, goMod string, mainGo string) string {
	t.Helper()
	tmpDir := t.TempDir()
	if err := os.WriteFile(path.Join(tmpDir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path.Join(tmpDir, "main.go"), []byte(mainGo), 0644); err != nil {
		t.Fatal(err)
	}

	origLoadPackages := loadPackages
	t.Cleanup(func() { loadPackages = origLoadPackages })

	loadPackages = func(cfg *packages.Config, paths ...string) ([]*packages.Package, error) {
		cfg.Dir = tmpDir
		return packages.Load(cfg, paths...)
	}

	return tmpDir
}

// check loads paths with checker and returns the unique unchecked errors in
// them. It fails the test if the packages can't be loaded or have errors.
func check(t *testing.T, checker *Checker, paths ...string) []UncheckedError {
	t.Helper()
	pkgs, err := checker.LoadPackages(paths...)
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
	return result.Unique().UncheckedErrors
}

// wantErrors is like check, but also fails the test unless there are exactly
// n unchecked errors.
func wantErrors(t *testing.T, checker *Checker, n int, paths ...string) []UncheckedError {
	t.Helper()
	errs := check(t, checker, paths...)
	if len(errs) != n {
		t.Errorf("expected %d errors, got %d: %v", n, len(errs), errs)
	}
	return errs
}

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
			wantErrors(t, &checker, test.numExpectedErrs, "github.com/testbuildtags")
		})
	}
}

func TestWhitelist(t *testing.T) {

}

func TestTypeParameterizedFunctionExclude(t *testing.T) {
	const testGoMod = `module typeparamtest

go 1.18
`
	const testLib = `package errtypes

func As[E error](err error) (E, bool) {
	e, ok := err.(E)
	return e, ok
}
`
	const testMain = `package main

import "typeparamtest/errtypes"

type MyError string

func (e MyError) Error() string { return string(e) }

func main() {
	var err error = MyError("test")
	_, ok := errtypes.As[MyError](err)
	_ = ok
}
`
	tmpDir := injectPackages(t, testGoMod, testMain)
	libDir := path.Join(tmpDir, "errtypes")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path.Join(libDir, "errtypes.go"), []byte(testLib), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("detected", func(t *testing.T) {
		var checker Checker
		checker.Exclusions.BlankAssignments = false
		wantErrors(t, &checker, 1, "typeparamtest")
	})

	t.Run("excluded", func(t *testing.T) {
		var checker Checker
		checker.Exclusions.BlankAssignments = false
		checker.Exclusions.Symbols = []string{"typeparamtest/errtypes.As"}
		wantErrors(t, &checker, 0, "typeparamtest")
	})
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
			wantErrors(t, &checker, test.numExpectedErrs, "github.com/testvendor")
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
			wantErrors(t, &checker, test.numExpectedErrs, "github.com/testvendor")
		})
	}
}

func test(t *testing.T, f flags) {
	var (
		asserts = f&CheckAsserts != 0
		blank   = f&CheckBlank != 0
	)
	var checker Checker
	checker.Exclusions.TypeAssertions = !asserts
	checker.Exclusions.BlankAssignments = !blank
	checker.Exclusions.Symbols = append(checker.Exclusions.Symbols, DefaultExcludedSymbols...)
	checker.Exclusions.Symbols = append(checker.Exclusions.Symbols,
		fmt.Sprintf("(%s.ErrorMakerInterface).MakeNilError", testPackage),
	)
	uerr := Result{UncheckedErrors: check(t, &checker, testPackage)}
	numErrors := len(uncheckedMarkers)
	if blank {
		numErrors += len(blankMarkers)
	}
	if asserts {
		numErrors += len(assertMarkers)
	}

	if len(uerr.UncheckedErrors) != numErrors {
		t.Errorf("got %d errors, want %d", len(uerr.UncheckedErrors), numErrors)
	uncheckedLoop:
		for k := range uncheckedMarkers {
			for _, e := range uerr.UncheckedErrors {
				if newMarker(e) == k {
					continue uncheckedLoop
				}
			}
			t.Errorf("Expected unchecked at %s", k)
		}
		if blank {
		blankLoop:
			for k := range blankMarkers {
				for _, e := range uerr.UncheckedErrors {
					if newMarker(e) == k {
						continue blankLoop
					}
				}
				t.Errorf("Expected blank at %s", k)
			}
		}
		if asserts {
		assertLoop:
			for k := range assertMarkers {
				for _, e := range uerr.UncheckedErrors {
					if newMarker(e) == k {
						continue assertLoop
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

func TestChannelReceive(t *testing.T) {
	const testGoMod = `module chantest

go 1.18
`
	const testMain = `package main

type CustomError string

func (e CustomError) Error() string { return string(e) }

func main() {
	errChan := make(chan error)
	customErrChan := make(chan CustomError)
	intChan := make(chan int)

	// Standalone receive expressions (discarded error)
	<-errChan
	<-customErrChan

	// Assigned receives (error used)
	err := <-errChan
	_ = err
	customErr := <-customErrChan
	_ = customErr

	// Blank assignments
	_ = <-errChan
	_, ok := <-errChan
	_ = ok

	// Non-error channels
	<-intChan
	v := <-intChan
	_ = v
	_ = <-intChan
	_, ok2 := <-intChan
	_ = ok2
}
`
	injectPackages(t, testGoMod, testMain)

	t.Run("default exclusions without blank checks", func(t *testing.T) {
		var checker Checker
		checker.Exclusions.BlankAssignments = true // ignore assignments to _
		wantErrors(t, &checker, 2, "chantest")
	})

	t.Run("with blank checks enabled", func(t *testing.T) {
		var checker Checker
		checker.Exclusions.BlankAssignments = false // check assignments to _
		wantErrors(t, &checker, 4, "chantest")
	})
}

func TestInjectPackages(t *testing.T) {
	const testMod = "module helpertest\n\ngo 1.18\n"
	const testMain = "package main\n\nfunc main() {}\n"

	var tmpDir string
	t.Run("injection and overrides", func(t *testing.T) {
		tmpDir = injectPackages(t, testMod, testMain)

		modBytes, err := os.ReadFile(path.Join(tmpDir, "go.mod"))
		if err != nil {
			t.Fatalf("failed to read go.mod: %v", err)
		}
		if string(modBytes) != testMod {
			t.Errorf("go.mod content mismatch: got %q, want %q", string(modBytes), testMod)
		}

		mainBytes, err := os.ReadFile(path.Join(tmpDir, "main.go"))
		if err != nil {
			t.Fatalf("failed to read main.go: %v", err)
		}
		if string(mainBytes) != testMain {
			t.Errorf("main.go content mismatch: got %q, want %q", string(mainBytes), testMain)
		}

		var cfg packages.Config
		_, _ = loadPackages(&cfg, "helpertest")
		if cfg.Dir != tmpDir {
			t.Errorf("expected cfg.Dir to be %q, got %q", tmpDir, cfg.Dir)
		}
	})

	var cfg packages.Config
	_, _ = loadPackages(&cfg, "helpertest")
	if cfg.Dir == tmpDir {
		t.Errorf("loadPackages was not restored after cleanup; cfg.Dir is still %q", cfg.Dir)
	}
}

func TestUnique(t *testing.T) {
	pos := func(file string, line, col int) token.Position {
		return token.Position{Filename: file, Line: line, Column: col}
	}
	a1 := UncheckedError{Pos: pos("a.go", 1, 1), Line: "f()", FuncName: "f"}
	a1g := UncheckedError{Pos: pos("a.go", 1, 1), Line: "f()", FuncName: "g"}
	a2 := UncheckedError{Pos: pos("a.go", 2, 1), Line: "f()", FuncName: "f"}
	b1 := UncheckedError{Pos: pos("b.go", 1, 1), Line: "f()", FuncName: "f"}

	r := Result{UncheckedErrors: []UncheckedError{b1, a1, a2, a1g, a1, b1}}
	orig := slices.Clone(r.UncheckedErrors)

	got := r.Unique().UncheckedErrors
	want := []UncheckedError{a1, a1g, a2, b1}
	if !slices.Equal(got, want) {
		t.Errorf("Unique() = %v, want %v", got, want)
	}
	if !slices.Equal(r.UncheckedErrors, orig) {
		t.Errorf("Unique() modified its receiver: %v, want %v", r.UncheckedErrors, orig)
	}
}
