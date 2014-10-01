package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/kisielk/errcheck/lib"
	"github.com/kisielk/gotool"
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
	return fmt.Sprintf("%q", strings.Join(pairs, ","))
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

var dotStar = regexp.MustCompile(".*")

func main() {
	ignore := ignoreFlag(map[string]*regexp.Regexp{
		"fmt": dotStar,
	})
	flag.Var(ignore, "ignore", "comma-separated list of pairs of the form pkg:regex\n"+
		"            the regex is used to ignore names within pkg")
	ignorePkg := flag.String("ignorepkg", "", "comma-separated list of package paths to ignore")
	blank := flag.Bool("blank", false, "if true, check for errors assigned to blank identifier")
	asserts := flag.Bool("asserts", false, "if true, check for ignored type assertion results")
	flag.Parse()

	for _, pkg := range strings.Split(*ignorePkg, ",") {
		if pkg != "" {
			ignore[pkg] = dotStar
		}
	}

	var pkgPaths = gotool.ImportPaths(flag.Args())
	if err := errcheck.CheckPackages(pkgPaths, ignore, *blank, *asserts); err != nil {
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
	os.Exit(0)
}
