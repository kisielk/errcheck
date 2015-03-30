package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
	os.Chdir(saveCwd)

	out := <-bufChannel

	if exitCode != exitUncheckedError {
		t.Errorf("Exit code is %d, expected %d", exitCode, exitUncheckedError)
	}

	expectUnchecked := 9
	if got := strings.Count(out, "UNCHECKED"); got != expectUnchecked {
		t.Errorf("Got %d UNCHECKED errors, expected %d in:\n%s", got, expectUnchecked, out)
	}
}

type parseTestCase struct {
	args    []string
	paths   []string
	ignore  map[string]string
	tags    []string
	blank   bool
	asserts bool
	error   int
}

func TestParseFlags(t *testing.T) {
	cases := []parseTestCase{
		parseTestCase{
			args:    []string{"errcheck"},
			paths:   []string{"."},
			ignore:  map[string]string{"fmt": dotStar.String()},
			tags:    []string{},
			blank:   false,
			asserts: false,
			error:   exitCodeOk,
		},
		parseTestCase{
			args:    []string{"errcheck", "-blank", "-asserts"},
			paths:   []string{"."},
			ignore:  map[string]string{"fmt": dotStar.String()},
			tags:    []string{},
			blank:   true,
			asserts: true,
			error:   exitCodeOk,
		},
		parseTestCase{
			args:    []string{"errcheck", "foo", "bar"},
			paths:   []string{"foo", "bar"},
			ignore:  map[string]string{"fmt": dotStar.String()},
			tags:    []string{},
			blank:   false,
			asserts: false,
			error:   exitCodeOk,
		},
		parseTestCase{
			args:    []string{"errcheck", "-ignore", "fmt:.*,encoding/binary:.*"},
			paths:   []string{"."},
			ignore:  map[string]string{"fmt": dotStar.String(), "encoding/binary": dotStar.String()},
			tags:    []string{},
			blank:   false,
			asserts: false,
			error:   exitCodeOk,
		},
		parseTestCase{
			args:    []string{"errcheck", "-ignore", "fmt:[FS]?[Pp]rint*"},
			paths:   []string{"."},
			ignore:  map[string]string{"fmt": "[FS]?[Pp]rint*"},
			tags:    []string{},
			blank:   false,
			asserts: false,
			error:   exitCodeOk,
		},
		parseTestCase{
			args:    []string{"errcheck", "-ignore", "[rR]ead|[wW]rite"},
			paths:   []string{"."},
			ignore:  map[string]string{"fmt": dotStar.String(), "": "[rR]ead|[wW]rite"},
			tags:    []string{},
			blank:   false,
			asserts: false,
			error:   exitCodeOk,
		},
		parseTestCase{
			args:    []string{"errcheck", "-ignorepkg", "testing"},
			paths:   []string{"."},
			ignore:  map[string]string{"fmt": dotStar.String(), "testing": dotStar.String()},
			tags:    []string{},
			blank:   false,
			asserts: false,
			error:   exitCodeOk,
		},
		parseTestCase{
			args:    []string{"errcheck", "-ignorepkg", "testing,foo"},
			paths:   []string{"."},
			ignore:  map[string]string{"fmt": dotStar.String(), "testing": dotStar.String(), "foo": dotStar.String()},
			tags:    []string{},
			blank:   false,
			asserts: false,
			error:   exitCodeOk,
		},
		parseTestCase{
			args:    []string{"errcheck", "-tags", "foo"},
			paths:   []string{"."},
			ignore:  map[string]string{"fmt": dotStar.String()},
			tags:    []string{"foo"},
			blank:   false,
			asserts: false,
			error:   exitCodeOk,
		},
		parseTestCase{
			args:    []string{"errcheck", "-tags", "foo bar !baz"},
			paths:   []string{"."},
			ignore:  map[string]string{"fmt": dotStar.String()},
			tags:    []string{"foo", "bar", "!baz"},
			blank:   false,
			asserts: false,
			error:   exitCodeOk,
		},
		parseTestCase{
			args:    []string{"errcheck", "-tags", "foo   bar   !baz"},
			paths:   []string{"."},
			ignore:  map[string]string{"fmt": dotStar.String()},
			tags:    []string{"foo", "bar", "!baz"},
			blank:   false,
			asserts: false,
			error:   exitCodeOk,
		},
	}

	assert := assert.New(t)
	for _, c := range cases {
		p, ign, t, b, a, e := parseFlags(c.args)

		i := map[string]string{}
		for k, v := range ign {
			i[k] = v.String()
		}

		argsStr := strings.Join(c.args, " ")
		assert.Equal(c.paths, p, "got expected paths from %q", argsStr)
		assert.Equal(c.ignore, i, "got expected ignore regexes from %q", argsStr)
		assert.Equal(c.tags, t, "got expected tags from %q", argsStr)
		assert.Equal(c.blank, b, "got expected blank flag setting from %q", argsStr)
		assert.Equal(c.asserts, a, "got expected asserts flag setting from %q", argsStr)
		assert.Equal(c.error, e, "got expected error code when parsing %q", argsStr)
	}
}
