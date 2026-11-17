package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/kisielk/errcheck/errcheck"
	"golang.org/x/tools/go/packages"
)

const (
	exitCodeOk int = iota
	exitUncheckedError
	exitFatalError
)

type ignoreFlag map[string]*regexp.Regexp

// version can be set at link time to override the version reported by errcheck
// (for example: -ldflags "-X main.version=v1.20.0").
var version string

func getVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}

// global flags
var (
	abspath bool
	verbose bool
)

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
	for pair := range strings.SplitSeq(s, ",") {
		pkg, re, ok := strings.Cut(pair, ":")
		if !ok {
			pkg, re = "", pair
		}
		regex, err := regexp.Compile(re)
		if err != nil {
			return err
		}
		f[pkg] = regex
	}
	return nil
}

type tagsFlag []string

func (f *tagsFlag) String() string {
	return fmt.Sprintf("%q", strings.Join(*f, ","))
}

func (f *tagsFlag) Set(s string) error {
	// FieldsFunc never returns empty fields, so repeated or leading
	// separators, and an empty s, add no tags.
	*f = append(*f, strings.FieldsFunc(s, func(c rune) bool {
		return c == ' ' || c == ','
	})...)
	return nil
}

func reportResult(e errcheck.Result) {
	wd, err := os.Getwd()
	if err != nil {
		wd = ""
	}
	for _, uncheckedError := range e.UncheckedErrors {
		pos := uncheckedError.Pos
		if !abspath {
			if rel, err := filepath.Rel(wd, pos.Filename); err == nil {
				pos.Filename = rel
			}
		}

		if verbose && uncheckedError.FuncName != "" {
			fmt.Printf("%s:\t%s\t%s\n", pos, uncheckedError.FuncName, uncheckedError.Line)
		} else {
			fmt.Printf("%s:\t%s\n", pos, uncheckedError.Line)
		}
	}
}

func logf(msg string, args ...any) {
	if verbose {
		_, _ = fmt.Fprintf(os.Stderr, msg+"\n", args...)
	}
}

func mainCmd(args []string) int {
	var checker errcheck.Checker
	paths, showVersion, err := parseFlags(&checker, args)
	switch {
	case errors.Is(err, flag.ErrHelp):
		return exitCodeOk // usage was printed
	case err != nil:
		return exitFatalError // the error was printed
	case showVersion:
		fmt.Printf("errcheck %s\n", getVersion())
		return exitCodeOk
	}

	result, err := checkPaths(&checker, paths...)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: failed to check packages: %s\n", err)
		return exitFatalError
	}
	if len(result.UncheckedErrors) > 0 {
		reportResult(result)
		return exitUncheckedError
	}
	return exitCodeOk
}

func checkPaths(c *errcheck.Checker, paths ...string) (errcheck.Result, error) {
	pkgs, err := c.LoadPackages(paths...)
	if err != nil {
		return errcheck.Result{}, err
	}
	// Check for errors in the initial packages.
	work := make(chan *packages.Package, len(pkgs))
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			return errcheck.Result{}, fmt.Errorf("errors while loading package %s: %v", pkg.ID, pkg.Errors)
		}
		work <- pkg
	}
	close(work)

	var wg sync.WaitGroup
	result := &errcheck.Result{}
	mu := &sync.Mutex{}
	for range runtime.NumCPU() {
		wg.Go(func() {
			for pkg := range work {
				logf("checking %s", pkg.Types.Path())
				r := c.CheckPackage(pkg)
				mu.Lock()
				result.Append(r)
				mu.Unlock()
			}
		})
	}

	wg.Wait()
	return result.Unique(), nil
}

// parseFlags parses command-line flags from args, setting them on checker.
// It returns the paths to check, defaulting to ".", and whether -version was
// given, in which case checker isn't set up. Errors are printed to stderr
// before they are returned; for -h or -help, the error is flag.ErrHelp and the
// usage is printed instead.
func parseFlags(checker *errcheck.Checker, args []string) (paths []string, showVersion bool, err error) {
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)

	var checkAsserts, checkBlanks bool

	flags.BoolVar(&checkBlanks, "blank", false, "if true, check for errors assigned to blank identifier")
	flags.BoolVar(&checkAsserts, "asserts", false, "if true, check for ignored type assertion results")
	flags.BoolVar(&checker.Exclusions.TestFiles, "ignoretests", false, "if true, checking of _test.go files is disabled")
	flags.BoolVar(&checker.Exclusions.GeneratedFiles, "ignoregenerated", false, "if true, checking of files with generated code is disabled")
	flags.BoolVar(&verbose, "verbose", false, "produce more verbose logging")

	flags.BoolVar(&abspath, "abspath", false, "print absolute paths to files")
	flags.BoolVar(&showVersion, "version", false, "print version and exit")

	tags := tagsFlag{}
	flags.Var(&tags, "tags", "comma or space-separated list of build tags to include")
	ignorePkg := flags.String("ignorepkg", "", "comma-separated list of package paths to ignore")
	ignore := ignoreFlag(map[string]*regexp.Regexp{})
	flags.Var(ignore, "ignore", "[deprecated] comma-separated list of pairs of the form pkg:regex\n"+
		"            the regex is used to ignore names within pkg.")

	var excludeFile string
	flags.StringVar(&excludeFile, "exclude", "", "Path to a file containing a list of functions to exclude from checking")

	var excludeOnly bool
	flags.BoolVar(&excludeOnly, "excludeonly", false, "Use only excludes from -exclude file")

	flags.StringVar(&checker.Mod, "mod", "", "module download mode to use: readonly or vendor. See 'go help modules' for more.")

	if err := flags.Parse(args[1:]); err != nil {
		return nil, false, err
	}
	if showVersion {
		return nil, true, nil
	}

	checker.Exclusions.BlankAssignments = !checkBlanks
	checker.Exclusions.TypeAssertions = !checkAsserts

	if !excludeOnly {
		checker.Exclusions.Symbols = append(checker.Exclusions.Symbols, errcheck.DefaultExcludedSymbols...)
	}

	if excludeFile != "" {
		excludes, err := errcheck.ReadExcludes(excludeFile)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Could not read exclude file: %v\n", err)
			return nil, false, err
		}
		checker.Exclusions.Symbols = append(checker.Exclusions.Symbols, excludes...)
	}

	checker.Tags = tags
	for pkg := range strings.SplitSeq(*ignorePkg, ",") {
		if pkg != "" {
			checker.Exclusions.Packages = append(checker.Exclusions.Packages, pkg)
		}
	}

	checker.Exclusions.SymbolRegexpsByPackage = ignore

	paths = flags.Args()
	if len(paths) == 0 {
		paths = []string{"."}
	}
	return paths, false, nil
}

func main() {
	os.Exit(mainCmd(os.Args))
}
