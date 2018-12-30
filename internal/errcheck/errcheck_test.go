package errcheck

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"runtime"
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
		Mode:  packages.LoadSyntax,
		Tests: true,
	}
	pkgs, err := packages.Load(cfg, testPackage)
	if err != nil {
		panic("failed to import test package")
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

	tmpGopath, err := ioutil.TempDir("", "testbuildtags")
	if err != nil {
		t.Fatalf("unable to create testbuildtags directory: %v", err)
	}
	testBuildTagsDir := path.Join(tmpGopath, "src", "github.com/testbuildtags")
	if err := os.MkdirAll(testBuildTagsDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	defer func() {
		os.RemoveAll(tmpGopath)
	}()

	if err := ioutil.WriteFile(path.Join(testBuildTagsDir, "custom1.go"), []byte(testBuildCustom1Tag), 0644); err != nil {
		t.Fatalf("Failed to write testbuildtags custom1: %v", err)
	}
	if err := ioutil.WriteFile(path.Join(testBuildTagsDir, "custom2.go"), []byte(testBuildCustom2Tag), 0644); err != nil {
		t.Fatalf("Failed to write testbuildtags custom2: %v", err)
	}
	if err := ioutil.WriteFile(path.Join(testBuildTagsDir, "doc.go"), []byte(testDoc), 0644); err != nil {
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

	for i, currCase := range cases {
		checker := NewChecker()
		checker.Tags = currCase.tags

		loadPackages = func(cfg *packages.Config, paths ...string) ([]*packages.Package, error) {
			cfg.Env = append(os.Environ(), "GOPATH="+tmpGopath)
			cfg.Dir = testBuildTagsDir
			pkgs, err := packages.Load(cfg, paths...)
			return pkgs, err
		}
		err := checker.CheckPackages("github.com/testbuildtags")

		if currCase.numExpectedErrs == 0 {
			if err != nil {
				t.Errorf("Case %d: expected no errors, but got: %v", i, err)
			}
			continue
		}

		uerr, ok := err.(*UncheckedErrors)
		if !ok {
			t.Errorf("Case %d: wrong error type returned: %v", i, err)
			continue
		}

		if currCase.numExpectedErrs != len(uerr.Errors) {
			t.Errorf("Case %d:\nExpected: %d errors\nActual:   %d errors", i, currCase.numExpectedErrs, len(uerr.Errors))
		}
	}
}

func TestWhitelist(t *testing.T) {

}

func TestIgnore(t *testing.T) {
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

	if strings.HasPrefix(runtime.Version(), "go1.5") && os.Getenv("GO15VENDOREXPERIMENT") != "1" {
		// skip tests if running in go1.5 and vendoring is not enabled
		t.SkipNow()
	}

	// copy testvendor directory into directory for test
	tmpGopath, err := ioutil.TempDir("", "testvendor")
	if err != nil {
		t.Fatalf("unable to create testvendor directory: %v", err)
	}
	testVendorDir := path.Join(tmpGopath, "src", "github.com/testvendor")
	if err := os.MkdirAll(testVendorDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	defer func() {
		os.RemoveAll(tmpGopath)
	}()

	if err := ioutil.WriteFile(path.Join(testVendorDir, "main.go"), []byte(testVendorMain), 0755); err != nil {
		t.Fatalf("Failed to write testvendor main: %v", err)
	}
	if err := os.MkdirAll(path.Join(testVendorDir, "vendor/github.com/testlog"), 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := ioutil.WriteFile(path.Join(testVendorDir, "vendor/github.com/testlog/testlog.go"), []byte(testLog), 0755); err != nil {
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

	for i, currCase := range cases {
		checker := NewChecker()
		checker.Ignore = currCase.ignore
		loadPackages = func(cfg *packages.Config, paths ...string) ([]*packages.Package, error) {
			cfg.Env = append(os.Environ(), "GOPATH="+tmpGopath)
			cfg.Dir = testVendorDir
			pkgs, err := packages.Load(cfg, paths...)
			return pkgs, err
		}
		err := checker.CheckPackages("github.com/testvendor")

		if currCase.numExpectedErrs == 0 {
			if err != nil {
				t.Errorf("Case %d: expected no errors, but got: %v", i, err)
			}
			continue
		}

		uerr, ok := err.(*UncheckedErrors)
		if !ok {
			t.Errorf("Case %d: wrong error type returned: %v", i, err)
			continue
		}

		if currCase.numExpectedErrs != len(uerr.Errors) {
			t.Errorf("Case %d:\nExpected: %d errors\nActual:   %d errors", i, currCase.numExpectedErrs, len(uerr.Errors))
		}
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
	const testLog = `
	package testlog

	func Info() error {
		return nil
	}`

	if strings.HasPrefix(runtime.Version(), "go1.5") && os.Getenv("GO15VENDOREXPERIMENT") != "1" {
		// skip tests if running in go1.5 and vendoring is not enabled
		t.SkipNow()
	}

	// copy testvendor directory into directory for test
	tmpGopath, err := ioutil.TempDir("", "testvendor")
	if err != nil {
		t.Fatalf("unable to create testvendor directory: %v", err)
	}
	testVendorDir := path.Join(tmpGopath, "src", "github.com/testvendor")
	if err := os.MkdirAll(testVendorDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	defer func() {
		os.RemoveAll(tmpGopath)
	}()

	if err := ioutil.WriteFile(path.Join(testVendorDir, "main.go"), []byte(testVendorMain), 0755); err != nil {
		t.Fatalf("Failed to write testvendor main: %v", err)
	}
	if err := os.MkdirAll(path.Join(testVendorDir, "vendor/github.com/testlog"), 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := ioutil.WriteFile(path.Join(testVendorDir, "vendor/github.com/testlog/testlog.go"), []byte(testLog), 0755); err != nil {
		t.Fatalf("Failed to write testlog: %v", err)
	}

	cases := []struct {
		withoutGeneratedCode bool
		numExpectedErrs      int
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
	}

	for i, currCase := range cases {
		checker := NewChecker()
		checker.WithoutGeneratedCode = currCase.withoutGeneratedCode
		loadPackages = func(cfg *packages.Config, paths ...string) ([]*packages.Package, error) {
			cfg.Env = append(os.Environ(), "GOPATH="+tmpGopath)
			cfg.Dir = testVendorDir
			pkgs, err := packages.Load(cfg, paths...)
			return pkgs, err
		}
		err := checker.CheckPackages(path.Join("github.com/testvendor"))

		if currCase.numExpectedErrs == 0 {
			if err != nil {
				t.Errorf("Case %d: expected no errors, but got: %v", i, err)
			}
			continue
		}

		uerr, ok := err.(*UncheckedErrors)
		if !ok {
			t.Errorf("Case %d: wrong error type returned: %v", i, err)
			continue
		}

		if currCase.numExpectedErrs != len(uerr.Errors) {
			t.Errorf("Case %d:\nExpected: %d errors\nActual:   %d errors", i, currCase.numExpectedErrs, len(uerr.Errors))
		}
	}
}

func test(t *testing.T, f flags) {
	var (
		asserts bool = f&CheckAsserts != 0
		blank   bool = f&CheckBlank != 0
	)
	checker := NewChecker()
	checker.Asserts = asserts
	checker.Blank = blank
	checker.SetExclude(map[string]bool{
		fmt.Sprintf("(%s.ErrorMakerInterface).MakeNilError", testPackage): true,
	})
	err := checker.CheckPackages(testPackage)
	uerr, ok := err.(*UncheckedErrors)
	if !ok {
		t.Fatalf("wrong error type returned: %v", err)
	}

	numErrors := len(uncheckedMarkers)
	if blank {
		numErrors += len(blankMarkers)
	}
	if asserts {
		numErrors += len(assertMarkers)
	}

	if len(uerr.Errors) != numErrors {
		t.Errorf("got %d errors, want %d", len(uerr.Errors), numErrors)
	unchecked_loop:
		for k := range uncheckedMarkers {
			for _, e := range uerr.Errors {
				if newMarker(e) == k {
					continue unchecked_loop
				}
			}
			t.Errorf("Expected unchecked at %s", k)
		}
		if blank {
		blank_loop:
			for k := range blankMarkers {
				for _, e := range uerr.Errors {
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
				for _, e := range uerr.Errors {
					if newMarker(e) == k {
						continue assert_loop
					}
				}
				t.Errorf("Expected assert at %s", k)
			}
		}
	}

	for i, err := range uerr.Errors {
		m := marker{err.Pos.Filename, err.Pos.Line}
		if !uncheckedMarkers[m] && !blankMarkers[m] && !assertMarkers[m] {
			t.Errorf("%d: unexpected error: %v", i, err)
		}
	}
}
