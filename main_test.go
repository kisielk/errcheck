package main

import (
	"bytes"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/kisielk/errcheck/errcheck"
)

var dotStar = regexp.MustCompile(".*")

func TestMain(t *testing.T) {
	saveStderr := os.Stderr
	saveStdout := os.Stdout
	saveCwd, err := os.Getwd()
	if err != nil {
		t.Errorf("Cannot receive current directory: %v", err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Errorf("Cannot create pipe: %v", err)
	}

	os.Stderr = w
	os.Stdout = w

	bufChannel := make(chan string)

	go func() {
		buf := new(bytes.Buffer)
		_, err = io.Copy(buf, r)
		r.Close()
		if err != nil {
			t.Errorf("Cannot copy to buffer: %v", err)
		}

		bufChannel <- buf.String()
	}()

	exitCode := mainCmd([]string{"cmd name", "github.com/kisielk/errcheck/testdata"})

	w.Close()

	os.Stderr = saveStderr
	os.Stdout = saveStdout
	_ = os.Chdir(saveCwd)

	out := <-bufChannel

	if exitCode != exitUncheckedError {
		t.Errorf("Exit code is %d, expected %d", exitCode, exitUncheckedError)
	}

	expectUnchecked := 29
	if got := strings.Count(out, "UNCHECKED"); got != expectUnchecked {
		t.Errorf("Got %d UNCHECKED errors, expected %d in:\n%s", got, expectUnchecked, out)
	}
}

type parseTestCase struct {
	args    []string
	paths   []string
	ignore  map[string]string // Exclusions.SymbolRegexpsByPackage
	pkgs    []string          // Exclusions.Packages
	tags    []string          // Tags
	blank   bool              // !BlankAssignments
	asserts bool              // !TypeAssertions
	error   int
}

func TestParseFlags(t *testing.T) {
	cases := []parseTestCase{
		parseTestCase{
			args:    []string{"errcheck"},
			paths:   []string{"."},
			ignore:  map[string]string{},
			pkgs:    []string{},
			tags:    []string{},
			blank:   false,
			asserts: false,
			error:   exitCodeOk,
		},
		parseTestCase{
			args:    []string{"errcheck", "-blank", "-asserts"},
			paths:   []string{"."},
			ignore:  map[string]string{},
			pkgs:    []string{},
			tags:    []string{},
			blank:   true,
			asserts: true,
			error:   exitCodeOk,
		},
		parseTestCase{
			args:    []string{"errcheck", "foo", "bar"},
			paths:   []string{"foo", "bar"},
			ignore:  map[string]string{},
			pkgs:    []string{},
			tags:    []string{},
			blank:   false,
			asserts: false,
			error:   exitCodeOk,
		},
		parseTestCase{
			args:    []string{"errcheck", "-ignore", "fmt:.*,encoding/binary:.*"},
			paths:   []string{"."},
			ignore:  map[string]string{"fmt": ".*", "encoding/binary": dotStar.String()},
			pkgs:    []string{},
			tags:    []string{},
			blank:   false,
			asserts: false,
			error:   exitCodeOk,
		},
		parseTestCase{
			args:    []string{"errcheck", "-ignore", "fmt:[FS]?[Pp]rint*"},
			paths:   []string{"."},
			ignore:  map[string]string{"fmt": "[FS]?[Pp]rint*"},
			pkgs:    []string{},
			tags:    []string{},
			blank:   false,
			asserts: false,
			error:   exitCodeOk,
		},
		parseTestCase{
			args:    []string{"errcheck", "-ignore", "[rR]ead|[wW]rite"},
			paths:   []string{"."},
			ignore:  map[string]string{"": "[rR]ead|[wW]rite"},
			pkgs:    []string{},
			tags:    []string{},
			blank:   false,
			asserts: false,
			error:   exitCodeOk,
		},
		parseTestCase{
			args:    []string{"errcheck", "-ignorepkg", "testing"},
			paths:   []string{"."},
			ignore:  map[string]string{},
			pkgs:    []string{"testing"},
			tags:    []string{},
			blank:   false,
			asserts: false,
			error:   exitCodeOk,
		},
		parseTestCase{
			args:    []string{"errcheck", "-ignorepkg", "testing,foo"},
			paths:   []string{"."},
			ignore:  map[string]string{},
			pkgs:    []string{"testing", "foo"},
			tags:    []string{},
			blank:   false,
			asserts: false,
			error:   exitCodeOk,
		},
		parseTestCase{
			args:    []string{"errcheck", "-tags", "foo"},
			paths:   []string{"."},
			ignore:  map[string]string{},
			pkgs:    []string{},
			tags:    []string{"foo"},
			blank:   false,
			asserts: false,
			error:   exitCodeOk,
		},
		parseTestCase{
			args:    []string{"errcheck", "-tags", "foo bar !baz"},
			paths:   []string{"."},
			ignore:  map[string]string{},
			pkgs:    []string{},
			tags:    []string{"foo", "bar", "!baz"},
			blank:   false,
			asserts: false,
			error:   exitCodeOk,
		},
		parseTestCase{
			args:    []string{"errcheck", "-tags", "foo,bar,!baz"},
			paths:   []string{"."},
			ignore:  map[string]string{},
			pkgs:    []string{},
			tags:    []string{"foo", "bar", "!baz"},
			blank:   false,
			asserts: false,
			error:   exitCodeOk,
		},
		parseTestCase{
			args:    []string{"errcheck", "-tags", "foo   bar   !baz"},
			paths:   []string{"."},
			ignore:  map[string]string{},
			pkgs:    []string{},
			tags:    []string{"foo", "bar", "!baz"},
			blank:   false,
			asserts: false,
			error:   exitCodeOk,
		},
	}

	slicesEqual := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}

	ignoresEqual := func(a map[string]*regexp.Regexp, b map[string]string) bool {
		if (a == nil && b != nil) || (a != nil && b == nil) || (len(a) != len(b)) {
			return false
		}
		for k, v := range a {
			if v.String() != b[k] {
				return false
			}
		}
		return true
	}

	for _, c := range cases {
		var checker errcheck.Checker
		p, e := parseFlags(&checker, c.args)

		argsStr := strings.Join(c.args, " ")
		if !slicesEqual(p, c.paths) {
			t.Errorf("%q: path got %q want %q", argsStr, p, c.paths)
		}
		if ign := checker.Exclusions.SymbolRegexpsByPackage; !ignoresEqual(ign, c.ignore) {
			t.Errorf("%q: ignore got %q want %q", argsStr, ign, c.ignore)
		}
		if pkgs := checker.Exclusions.Packages; !slicesEqual(pkgs, c.pkgs) {
			t.Errorf("%q: packages got %v want %v", argsStr, pkgs, c.pkgs)
		}
		if tags := checker.Tags; !slicesEqual(tags, c.tags) {
			t.Errorf("%q: tags got %v want %v", argsStr, tags, c.tags)
		}
		if b := checker.Exclusions.BlankAssignments; b != !c.blank {
			t.Errorf("%q: BlankAssignments got %v want %v", argsStr, b, !c.blank)
		}
		if a := checker.Exclusions.TypeAssertions; a != !c.asserts {
			t.Errorf("%q: TypeAssertions got %v want %v", argsStr, a, !c.asserts)
		}
		if e != c.error {
			t.Errorf("%q: error got %q want %q", argsStr, e, c.error)
		}
	}
}
