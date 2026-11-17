package errcheck

import (
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

const testPackage = "github.com/kisielk/errcheck/testdata"

// Marker comments in testPackage, put on lines where an error should be
// reported. Blank and assert markers only apply when those checks are on.
const (
	markUnchecked = "UNCHECKED"
	markBlank     = "BLANK"
	markAssert    = "ASSERT"
)

// markers maps each marked line in testPackage to its marker.
var markers = map[marker]string{}

// marker is a line in a file.
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
				switch text := strings.TrimSuffix(comment.Text(), "\n"); text {
				case markUnchecked, markBlank, markAssert:
					pos := pkg.Fset.Position(comment.Pos())
					markers[marker{pos.Filename, pos.Line}] = text
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

// useTempModule writes files to a temporary directory, and overrides
// loadPackages to load packages from that directory for the duration of the
// test. The keys of files are slash-separated paths relative to the directory,
// such as "go.mod" or "sub/sub.go", and parent directories are created as
// needed. If env is given, it is added to the environment packages are loaded
// with. It returns the path to the temporary directory.
func useTempModule(t *testing.T, files map[string]string, env ...string) string {
	t.Helper()
	tmpDir := t.TempDir()
	for name, content := range files {
		file := filepath.Join(tmpDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	origLoadPackages := loadPackages
	t.Cleanup(func() { loadPackages = origLoadPackages })

	loadPackages = func(cfg *packages.Config, paths ...string) ([]*packages.Package, error) {
		cfg.Dir = tmpDir
		if len(env) > 0 {
			cfg.Env = append(os.Environ(), env...)
		}
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

	useTempModule(t, map[string]string{
		"go.mod":     "module github.com/testbuildtags",
		"custom1.go": testBuildCustom1Tag,
		"custom2.go": testBuildCustom2Tag,
		"doc.go":     testDoc,
	}, "GOPATH="+t.TempDir())

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
	useTempModule(t, map[string]string{
		"go.mod":               testGoMod,
		"main.go":              testMain,
		"errtypes/errtypes.go": testLib,
	})

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

// testVendorModule returns the files of the module github.com/testvendor,
// with mainGo as its main.go. It vendors a package github.com/testlog whose
// Info function returns an error.
func testVendorModule(mainGo string) map[string]string {
	return map[string]string{
		"go.mod": `module github.com/testvendor

require github.com/testlog v0.0.0
`,
		"main.go": mainGo,
		"vendor/github.com/testlog/testlog.go": `
	package testlog

	func Info() error {
		return nil
	}`,
	}
}

func TestIgnore(t *testing.T) {
	const testVendorMain = `
	package main

	import "github.com/testlog"

	func main() {
		// returns an error that is not checked
		testlog.Info()
	}`
	useTempModule(t, testVendorModule(testVendorMain), "GOPATH="+t.TempDir(), "GOFLAGS=-mod=vendor")

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
				"github.com/testvendor/vendor/github.com/testlog": regexp.MustCompile("Info"),
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
			wantErrors(t, &checker, test.numExpectedErrs, "github.com/testvendor")
		})
	}
}

func TestWithoutGeneratedCode(t *testing.T) {
	const testVendorMain = `
	// Code generated by protoc-gen-go. DO NOT EDIT.
	package main

	import "github.com/testlog"

	func main() {
		// returns an error that is not checked
		testlog.Info()
	}`

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
			// Without checker.Mod, select the vendor directory through GOFLAGS.
			env := []string{"GOPATH=" + t.TempDir()}
			if !test.withModVendor {
				env = append(env, "GOFLAGS=-mod=vendor")
			}
			useTempModule(t, testVendorModule(testVendorMain), env...)
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
	// The marked lines that should be reported with these flags.
	want := map[marker]string{}
	for m, kind := range markers {
		if kind == markUnchecked || (blank && kind == markBlank) || (asserts && kind == markAssert) {
			want[m] = kind
		}
	}

	// Each wanted line must be reported exactly once, and no other line.
	got := map[marker]int{}
	for _, err := range check(t, &checker, testPackage) {
		m := newMarker(err)
		got[m]++
		if _, ok := want[m]; !ok {
			t.Errorf("unexpected error: %v", err)
		}
		if err.SelectorName != "" && !strings.Contains(err.Line, err.SelectorName) {
			t.Errorf("the line '%s' must contain the selector '%s'", err.Line, err.SelectorName)
		}
	}
	for m, kind := range want {
		if got[m] != 1 {
			t.Errorf("expected one error at %s (%s), got %d", m, kind, got[m])
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
	useTempModule(t, map[string]string{"go.mod": testGoMod, "main.go": testMain})

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

func TestUseTempModule(t *testing.T) {
	files := map[string]string{
		"go.mod":     "module helpertest\n\ngo 1.18\n",
		"main.go":    "package main\n\nfunc main() {}\n",
		"sub/sub.go": "package sub\n",
	}

	var tmpDir string
	t.Run("injection and overrides", func(t *testing.T) {
		tmpDir = useTempModule(t, files)

		for name, want := range files {
			got, err := os.ReadFile(filepath.Join(tmpDir, filepath.FromSlash(name)))
			if err != nil {
				t.Fatalf("failed to read %s: %v", name, err)
			}
			if string(got) != want {
				t.Errorf("%s content mismatch: got %q, want %q", name, got, want)
			}
		}

		var cfg packages.Config
		_, _ = loadPackages(&cfg, "helpertest")
		if cfg.Dir != tmpDir {
			t.Errorf("expected cfg.Dir to be %q, got %q", tmpDir, cfg.Dir)
		}
		if cfg.Env != nil {
			t.Errorf("expected cfg.Env to be unset without env, got %d entries", len(cfg.Env))
		}
	})

	t.Run("environment", func(t *testing.T) {
		useTempModule(t, files, "HELPERTEST=1")

		var cfg packages.Config
		_, _ = loadPackages(&cfg, "helpertest")
		if !slices.Contains(cfg.Env, "HELPERTEST=1") {
			t.Errorf("expected cfg.Env to contain HELPERTEST=1")
		}
		if len(cfg.Env) != len(os.Environ())+1 {
			t.Errorf("expected cfg.Env to be the environment plus HELPERTEST=1, got %d entries", len(cfg.Env))
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
