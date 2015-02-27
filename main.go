package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"

	"github.com/kisielk/errcheck/internal"
	"github.com/kisielk/gotool"
)

const (
	exitCodeOk int = iota
	exitUncheckedError
	exitFatalError
)

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

func mainCmd(args []string) int {
	runtime.GOMAXPROCS(runtime.NumCPU())

	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)

	ignore := ignoreFlag(map[string]*regexp.Regexp{
		"fmt": dotStar,
	})
	flags.Var(ignore, "ignore", "comma-separated list of pairs of the form pkg:regex\n"+
		"            the regex is used to ignore names within pkg")
	ignorePkg := flags.String("ignorepkg", "", "comma-separated list of package paths to ignore")
	blank := flags.Bool("blank", false, "if true, check for errors assigned to blank identifier")
	asserts := flags.Bool("asserts", false, "if true, check for ignored type assertion results")

	if err := flags.Parse(args[1:]); err != nil {
		return exitFatalError
	}

	for _, pkg := range strings.Split(*ignorePkg, ",") {
		if pkg != "" {
			ignore[pkg] = dotStar
		}
	}

	// ImportPaths normalizes paths and expands '...'
	var expandedArgs = gotool.ImportPaths(flags.Args())
	if err := errcheck.CheckPackages(expandedArgs, ignore, *blank, *asserts); err != nil {
		if e, ok := err.(errcheck.UncheckedErrors); ok {
			for _, uncheckedError := range e.Errors {
				fmt.Println(uncheckedError)
			}
			return exitUncheckedError
		} else if err == errcheck.ErrNoGoFiles {
			fmt.Fprintln(os.Stderr, err)
			return exitCodeOk
		}
		fmt.Fprintf(os.Stderr, "error: failed to check packages: %s\n", err)
		return exitFatalError
	}
	return exitCodeOk
}

func main() {
	os.Exit(mainCmd(os.Args))
}
