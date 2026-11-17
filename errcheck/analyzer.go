package errcheck

import (
	"fmt"
	"go/ast"
	"reflect"
	"regexp"
	"sync"

	"golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
	Name:       "errcheck",
	Doc:        "check for unchecked errors",
	Run:        runAnalyzer,
	ResultType: reflect.TypeFor[Result](),
}

var (
	argBlank       bool
	argAsserts     bool
	argExcludeFile string
	argExcludeOnly bool
)

func init() {
	Analyzer.Flags.BoolVar(&argBlank, "blank", false, "if true, check for errors assigned to blank identifier")
	Analyzer.Flags.BoolVar(&argAsserts, "asserts", false, "if true, check for ignored type assertion results")
	Analyzer.Flags.BoolVar(&argAsserts, "assert", false, "deprecated: use -asserts")
	Analyzer.Flags.StringVar(&argExcludeFile, "exclude", "", "Path to a file containing a list of functions to exclude from checking")
	Analyzer.Flags.BoolVar(&argExcludeOnly, "excludeonly", false, "Use only excludes from -exclude file")
}

// excludeCache holds the excluded symbols built from the flags, so the
// exclude file is read once rather than once per package. It is keyed by
// the flags it was built from, since tests change them between runs.
var excludeCache struct {
	sync.Mutex
	file    string
	only    bool
	symbols map[string]bool // nil until built
	err     error
}

// excludedSymbols returns the set of symbols to exclude, from the defaults
// unless -excludeonly is set, and from the -exclude file if there is one.
// Callers must not modify the returned map, since it is shared.
func excludedSymbols() (map[string]bool, error) {
	c := &excludeCache
	c.Lock()
	defer c.Unlock()
	if (c.symbols != nil || c.err != nil) && c.file == argExcludeFile && c.only == argExcludeOnly {
		// Current flags match the cache, return the cached version.
		return c.symbols, c.err
	}

	// Rebuild the cache from the current flags.
	c.file, c.only = argExcludeFile, argExcludeOnly
	c.symbols, c.err = buildExcludedSymbols(argExcludeFile, argExcludeOnly)
	return c.symbols, c.err
}

func buildExcludedSymbols(file string, only bool) (map[string]bool, error) {
	exclude := map[string]bool{}
	if !only {
		for _, name := range DefaultExcludedSymbols {
			exclude[name] = true
		}
	}
	if file != "" {
		excludes, err := ReadExcludes(file)
		if err != nil {
			return nil, fmt.Errorf("could not read exclude file: %w", err)
		}
		for _, name := range excludes {
			exclude[name] = true
		}
	}
	return exclude, nil
}

func runAnalyzer(pass *analysis.Pass) (any, error) {
	exclude, err := excludedSymbols()
	if err != nil {
		return nil, err
	}

	var allErrors []UncheckedError
	for _, f := range pass.Files {
		v := &visitor{
			typesInfo:             pass.TypesInfo,
			fset:                  pass.Fset,
			checkBlankAssignments: argBlank,
			checkTypeAsserts:      argAsserts,
			exclude:               exclude,
			ignore:                map[string]*regexp.Regexp{}, // deprecated & not used
			lines:                 make(map[string][]string),
			errors:                nil,
		}

		ast.Walk(v, f)

		for _, err := range v.errors {
			pass.Report(analysis.Diagnostic{
				Pos:      pass.Fset.File(f.Pos()).Pos(err.Pos.Offset),
				Message:  "unchecked error",
				Category: "errcheck",
			})
		}

		allErrors = append(allErrors, v.errors...)
	}

	return Result{UncheckedErrors: allErrors}, nil
}
