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

func TestMainFunc(t *testing.T) {
	var exitCode int
	out := captureOutput(t, func() {
		exitCode = mainCmd([]string{"cmd name", "github.com/kisielk/errcheck/testdata"})
	})

	if exitCode != exitUncheckedError {
		t.Errorf("Exit code is %d, expected %d", exitCode, exitUncheckedError)
	}

	expectUnchecked := 49
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
	version bool              // -version given
}

func TestParseFlags(t *testing.T) {
	cases := []parseTestCase{
		{
			args:    []string{"errcheck"},
			paths:   []string{"."},
			ignore:  map[string]string{},
			pkgs:    []string{},
			tags:    []string{},
			blank:   false,
			asserts: false,
		},
		{
			args:    []string{"errcheck", "-blank", "-asserts"},
			paths:   []string{"."},
			ignore:  map[string]string{},
			pkgs:    []string{},
			tags:    []string{},
			blank:   true,
			asserts: true,
		},
		{
			args:    []string{"errcheck", "foo", "bar"},
			paths:   []string{"foo", "bar"},
			ignore:  map[string]string{},
			pkgs:    []string{},
			tags:    []string{},
			blank:   false,
			asserts: false,
		},
		{
			args:    []string{"errcheck", "-ignore", "fmt:.*,encoding/binary:.*"},
			paths:   []string{"."},
			ignore:  map[string]string{"fmt": ".*", "encoding/binary": dotStar.String()},
			pkgs:    []string{},
			tags:    []string{},
			blank:   false,
			asserts: false,
		},
		{
			args:    []string{"errcheck", "-ignore", "fmt:[FS]?[Pp]rint*"},
			paths:   []string{"."},
			ignore:  map[string]string{"fmt": "[FS]?[Pp]rint*"},
			pkgs:    []string{},
			tags:    []string{},
			blank:   false,
			asserts: false,
		},
		{
			args:    []string{"errcheck", "-ignore", "[rR]ead|[wW]rite"},
			paths:   []string{"."},
			ignore:  map[string]string{"": "[rR]ead|[wW]rite"},
			pkgs:    []string{},
			tags:    []string{},
			blank:   false,
			asserts: false,
		},
		{
			args:    []string{"errcheck", "-ignorepkg", "testing"},
			paths:   []string{"."},
			ignore:  map[string]string{},
			pkgs:    []string{"testing"},
			tags:    []string{},
			blank:   false,
			asserts: false,
		},
		{
			args:    []string{"errcheck", "-ignorepkg", "testing,foo"},
			paths:   []string{"."},
			ignore:  map[string]string{},
			pkgs:    []string{"testing", "foo"},
			tags:    []string{},
			blank:   false,
			asserts: false,
		},
		{
			args:    []string{"errcheck", "-tags", "foo"},
			paths:   []string{"."},
			ignore:  map[string]string{},
			pkgs:    []string{},
			tags:    []string{"foo"},
			blank:   false,
			asserts: false,
		},
		{
			args:    []string{"errcheck", "-tags", "foo bar !baz"},
			paths:   []string{"."},
			ignore:  map[string]string{},
			pkgs:    []string{},
			tags:    []string{"foo", "bar", "!baz"},
			blank:   false,
			asserts: false,
		},
		{
			args:    []string{"errcheck", "-tags", "foo,bar,!baz"},
			paths:   []string{"."},
			ignore:  map[string]string{},
			pkgs:    []string{},
			tags:    []string{"foo", "bar", "!baz"},
			blank:   false,
			asserts: false,
		},
		{
			args:    []string{"errcheck", "-tags", "foo   bar   !baz"},
			paths:   []string{"."},
			ignore:  map[string]string{},
			pkgs:    []string{},
			tags:    []string{"foo", "bar", "!baz"},
			blank:   false,
			asserts: false,
		},
		{
			args:    []string{"errcheck", "-version"},
			version: true, // the checker isn't set up, and no paths are returned
		},
		{
			args:    []string{"errcheck", "-version", "foo"},
			version: true, // the checker isn't set up, and no paths are returned
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
		p, v, err := parseFlags(&checker, c.args)

		argsStr := strings.Join(c.args, " ")
		if err != nil {
			t.Errorf("%q: unexpected error %v", argsStr, err)
		}
		if v != c.version {
			t.Errorf("%q: version got %v want %v", argsStr, v, c.version)
		}
		if !slicesEqual(p, c.paths) {
			t.Errorf("%q: path got %q want %q", argsStr, p, c.paths)
		}
		if c.version {
			continue
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
	}
}

func captureOutput(t *testing.T, f func()) string {
	t.Helper()
	saveStderr := os.Stderr
	saveStdout := os.Stdout
	defer func() {
		os.Stderr = saveStderr
		os.Stdout = saveStdout
	}()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Cannot create pipe: %v", err)
	}

	os.Stderr = w
	os.Stdout = w

	bufChannel := make(chan string)
	go func() {
		buf := new(bytes.Buffer)
		_, _ = io.Copy(buf, r)
		_ = r.Close()
		bufChannel <- buf.String()
	}()

	f()
	_ = w.Close()

	return <-bufChannel
}

func TestVersion(t *testing.T) {
	var exitCode int
	out := captureOutput(t, func() {
		exitCode = mainCmd([]string{"errcheck", "-version"})
	})

	if exitCode != exitCodeOk {
		t.Errorf("Exit code is %d, expected %d", exitCode, exitCodeOk)
	}

	expectedPrefix := "errcheck "
	if !strings.HasPrefix(out, expectedPrefix) {
		t.Errorf("Output %q does not start with %q", out, expectedPrefix)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("Output %q does not end with newline", out)
	}
}

func TestVersionCustom(t *testing.T) {
	oldVersion := version
	defer func() {
		version = oldVersion
	}()
	version = "v1.20.0"

	var exitCode int
	out := captureOutput(t, func() {
		exitCode = mainCmd([]string{"errcheck", "-version"})
	})

	if exitCode != exitCodeOk {
		t.Errorf("Exit code is %d, expected %d", exitCode, exitCodeOk)
	}

	expected := "errcheck v1.20.0\n"
	if out != expected {
		t.Errorf("Output is %q, expected %q", out, expected)
	}
}

// TestHelp checks that -h prints usage and exits successfully, as programs
// using the flag package's ExitOnError do.
func TestHelp(t *testing.T) {
	for _, arg := range []string{"-h", "-help"} {
		var exitCode int
		out := captureOutput(t, func() {
			exitCode = mainCmd([]string{"errcheck", arg})
		})
		if exitCode != exitCodeOk {
			t.Errorf("%s: exit code is %d, expected %d", arg, exitCode, exitCodeOk)
		}
		if !strings.Contains(out, "Usage of errcheck") {
			t.Errorf("%s: output %q does not contain usage", arg, out)
		}
	}
}

// TestFlagErrors checks that bad flags and a missing exclude file are
// reported, and exit with exitFatalError.
func TestFlagErrors(t *testing.T) {
	cases := []struct {
		args []string
		out  string
	}{
		{[]string{"errcheck", "-nosuchflag"}, "flag provided but not defined"},
		{[]string{"errcheck", "-exclude", "testdata/no_such_file"}, "Could not read exclude file"},
	}
	for _, c := range cases {
		var exitCode int
		out := captureOutput(t, func() {
			exitCode = mainCmd(c.args)
		})
		if exitCode != exitFatalError {
			t.Errorf("%q: exit code is %d, expected %d", c.args, exitCode, exitFatalError)
		}
		if !strings.Contains(out, c.out) {
			t.Errorf("%q: output %q does not contain %q", c.args, out, c.out)
		}
	}
}
