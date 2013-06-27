package main

import (
	"flag"
	"fmt"
	"github.com/kisielk/errcheck/lib"
	"github.com/kisielk/gotool"
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

type ignoreFlag map[string]*regexp.Regexp

func (f ignoreFlag) String() string {
	pairs := make([]string, 0, len(f))
	for pkg, re := range f {
		prefix := ""
		if pkg != "" {
			prefix = pkg + ":"
		}
		pairs = append(pairs, prefix+re.String())
	}
	return strings.Join(pairs, ",")
}

func (f ignoreFlag) Set(s string) error {
	if s == "" {
		return nil
	}
	for _, pair := range strings.Split(s, ",") {
		colonIndex := strings.Index(pair, ":")
		var pkg, re string
		if colonIndex == -1 {
			pkg = ""
			re = pair
		} else {
			pkg = pair[:colonIndex]
			re = pair[colonIndex+1:]
		}
		regex, err := regexp.Compile(re)
		if err != nil {
			return err
		}
		f[pkg] = regex
	}
	return nil
}

func main() {
	dotStar := regexp.MustCompile(".*")

	ignore := ignoreFlag(make(map[string]*regexp.Regexp))
	flag.Var(ignore, "ignore", "comma-separated list of pairs of the form pkg:regex\n"+
		"            the regex is used to ignore names within pkg")
	ignorePkg := flag.String("ignorepkg", "", "comma-separated list of package paths to ignore")
	blank := flag.Bool("blank", false, "if true, check for errors assigned to blank identifier")
	flag.Parse()

	for _, pkg := range strings.Split(*ignorePkg, ",") {
		if pkg != "" {
			ignore[pkg] = dotStar
		}
	}

	if _, ok := ignore["fmt"]; !ok {
		ignore["fmt"] = dotStar
	}

	pattern := flag.Arg(0)
	if pattern == "" {
		flag.Usage()
		Fatalf("you must specify a package pattern")
	}

	var exitStatus int
	for _, pkgPath := range gotool.MatchPackages(pattern) {
		if err := errcheck.CheckPackage(pkgPath, ignore, *blank); err != nil {
			if e, ok := err.(errcheck.UncheckedErrors); ok {
				for _, uncheckedError := range e.Errors {
					fmt.Println(uncheckedError)
				}
				exitStatus = 1
				continue
			} else if err == errcheck.ErrNoGoFiles {
				fmt.Fprintln(os.Stderr, err)
				continue
			}
			Fatalf("failed to check package %s: %s", pkgPath, err)
		}
	}
	os.Exit(exitStatus)
}
