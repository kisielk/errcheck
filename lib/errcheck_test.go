package errcheck

import (
	"regexp"
	"strings"
	"testing"
)

func Test(t *testing.T) {
	expectedErrors := []struct {
		pos  string
		line string
	}{
		{"github.com/kisielk/errcheck/example/main.go:6:13", `fmt.Println("this function returns an error")`},
		{"github.com/kisielk/errcheck/example/main.go:11:13", `fmt.Println("this function returns an int and an error")`},
		{"github.com/kisielk/errcheck/example/main.go:17:2", `_ = a()`},
		{"github.com/kisielk/errcheck/example/main.go:18:3", `a()`},
		{"github.com/kisielk/errcheck/example/main.go:21:5", `_, _ = b()`},
		{"github.com/kisielk/errcheck/example/main.go:22:3", `b()`},
		{"github.com/kisielk/errcheck/example/main.go:26:2", `_ = x.a()`},
		{"github.com/kisielk/errcheck/example/main.go:27:5", `x.a()`},
		{"github.com/kisielk/errcheck/example/main.go:31:2", `_ = y.t.a()`},
		{"github.com/kisielk/errcheck/example/main.go:32:7", `y.t.a()`},
		{"github.com/kisielk/errcheck/example/main.go:35:2", `_ = m1["a"]()`},
		{"github.com/kisielk/errcheck/example/main.go:36:9", `m1["a"]()`},
		{"github.com/kisielk/errcheck/example/main.go:39:5", `z, _ := b()`},
		{"github.com/kisielk/errcheck/example/main.go:40:2", `_, w := a(), 5`},
		{"github.com/kisielk/errcheck/example/main2.go:9:13", `fmt.Println("this method returns an error")`},
	}

	err := CheckPackage("github.com/kisielk/errcheck/example", make(map[string]*regexp.Regexp), true)
	uerr, ok := err.(UncheckedErrors)
	if !ok {
		t.Fatal("wrong error type returned")
	}

	if len(uerr.Errors) != 15 {
		t.Errorf("got %d errors, want 15", len(uerr.Errors))
		for i, err := range uerr.Errors {
			t.Errorf("%d: %v", i, err)
		}
		return
	}

	for i, err := range uerr.Errors {
		uerr, ok := err.(uncheckedError)
		if !ok {
			t.Errorf("%d: not an uncheckedError, got %v", i, err)
			continue
		}

		expected := expectedErrors[i]
		if !strings.HasSuffix(uerr.pos.String(), expected.pos) {
			t.Errorf("%d: wrong position: got %q, want %q", i, uerr.pos.String(), expected.pos)
		}
		if errLine := string(uerr.line); errLine != expected.line {
			t.Errorf("%d: wrong line: got %q, want %q", i, errLine, expected.line)
		}
	}
}
