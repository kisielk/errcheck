package main

import (
	"flag"
	"fmt"
	"github.com/kisielk/errcheck/lib"
	"os"
	"regexp"
	"strings"
)

// Err prints an error to Stderr
func Err(s string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "error: "+s+"\n", args...)
}

// Fatal calls Err followed by os.Exit(2)
func Fatalf(s string, args ...interface{}) {
	Err(s, args...)
	os.Exit(2)
}

// regexpFlag is a type that can be used with flag.Var for regular expression flags
type regexpFlag struct {
	re *regexp.Regexp
}

func (r regexpFlag) String() string {
	if r.re == nil {
		return ""
	}
	return r.re.String()
}

func (r *regexpFlag) Set(s string) error {
	if s == "" {
		r.re = nil
		return nil
	}

	re, err := regexp.Compile(s)
	if err != nil {
		return err
	}
	r.re = re
	return nil
}

// stringsFlag is a type that can be used with flag.Var for lists that are turned to a set
type stringsFlag struct {
	items map[string]bool
}

func (f stringsFlag) String() string {
	items := make([]string, 0, len(f.items))
	for k := range f.items {
		items = append(items, k)
	}
	return strings.Join(items, ",")
}

func (f *stringsFlag) Set(s string) error {
	f.items = make(map[string]bool)
	for _, item := range strings.Split(s, ",") {
		f.items[item] = true
	}
	return nil
}

func main() {
	var ignore regexpFlag
	flag.Var(&ignore, "ignore", "regular expression of function names to ignore")
	ignorePkg := &stringsFlag{}
	ignorePkg.Set("fmt")
	flag.Var(ignorePkg, "ignorepkg", "comma-separated list of package paths to ignore")
	blank := flag.Bool("blank", false, "if true, check for errors assigned to blank identifier")
	flag.Parse()

	pkgPath := flag.Arg(0)
	if pkgPath == "" {
		flag.Usage()
		Fatalf("you must specify a package")
	}

	if err := errcheck.CheckPackage(pkgPath, ignore.re, ignorePkg.items, *blank); err != nil {
		if e, ok := err.(errcheck.UncheckedErrors); ok {
			for _, uncheckedError := range e.Errors {
				fmt.Println(uncheckedError)
			}
			os.Exit(1)
		} else if err == errcheck.ErrNoGoFiles {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(0)
		}
		Fatalf("failed to check package: %s", err)
	}
}
