// Package errcheck is the library used to implement the errcheck command-line tool.
//
// Note: The API of this package has not been finalized and may change at any point.
package errcheck

import (
	"bufio"
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/token"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"

	"golang.org/x/tools/go/loader"
	"golang.org/x/tools/go/types"
)

var (
	// ErrNoGoFiles is returned when CheckPackage is run on a package with no Go source files
	ErrNoGoFiles = errors.New("package contains no go source files")
)

// UncheckedErrors is returned from the CheckPackage function if the package contains
// any unchecked errors.
type UncheckedErrors struct {
	// Errors is a list of all the unchecked errors in the package.
	// Printing an error reports its position within the file and the contents of the line.
	Errors []error
}

func (e UncheckedErrors) Error() string {
	return fmt.Sprintf("%d unchecked errors", len(e.Errors))
}

// Len is the number of elements in the collection.
func (e UncheckedErrors) Len() int { return len(e.Errors) }

// Swap swaps the elements with indexes i and j.
func (e UncheckedErrors) Swap(i, j int) { e.Errors[i], e.Errors[j] = e.Errors[j], e.Errors[i] }

type byName struct{ UncheckedErrors }

// Less reports whether the element with index i should sort before the element with index j.
func (e byName) Less(i, j int) bool {
	ei, ej := e.Errors[i].(uncheckedError), e.Errors[j].(uncheckedError)

	pi, pj := ei.pos, ej.pos

	if pi.Filename != pj.Filename {
		return pi.Filename < pj.Filename
	}
	if pi.Line != pj.Line {
		return pi.Line < pj.Line
	}
	if pi.Column != pj.Column {
		return pi.Column < pj.Column
	}

	return ei.line < ej.line
}

// CheckPackages checks packages for errors.
// ignore is a map of package names to regular expressions. Identifiers from a package are
// checked against its regular expressions and if any of the expressions match the call
// is not checked.
// tags is a slice of build tags (as string) to include. This list can be empty.
// If blank is true then assignments to the blank identifier are also considered to be
// ignored errors.
// If asserts is true then ignored type assertion results are also checked
func CheckPackages(args []string, ignore map[string]*regexp.Regexp, tags []string, blank bool, asserts bool) error {
	ctx := build.Default
	if len(tags) > 0 {
		for _, tag := range tags {
			ctx.BuildTags = append(ctx.BuildTags, tag)
		}
	}
	loadcfg := loader.Config{
		ImportFromBinary: false,
		Build:            &ctx,
	}
	rest, err := loadcfg.FromArgs(args, true)
	if err != nil {
		return fmt.Errorf("could not parse arguments: %s", err)
	}
	if len(rest) > 0 {
		return fmt.Errorf("unhandled extra arguments: %v", rest)
	}

	program, err := loadcfg.Load()
	if err != nil {
		return fmt.Errorf("could not type check: %s", err)
	}

	var errsMutex sync.Mutex
	var errs []error

	var wg sync.WaitGroup

	for _, pkgInfo := range program.InitialPackages() {
		if pkgInfo.Pkg.Path() == "unsafe" { // not a real package
			continue
		}

		wg.Add(1)

		go func(pkgInfo *loader.PackageInfo) {
			defer wg.Done()

			visitor := &checker{program, pkgInfo, ignore, blank, asserts, make(map[string][]string), []error{}}

			for _, astFile := range visitor.pkg.Files {
				ast.Walk(visitor, astFile)
			}

			if len(visitor.errors) > 0 {
				errsMutex.Lock()
				defer errsMutex.Unlock()

				errs = append(errs, visitor.errors...)
			}
		}(pkgInfo)
	}

	wg.Wait()

	if len(errs) > 0 {
		u := UncheckedErrors{errs}

		sort.Sort(byName{u})

		return u
	}

	return nil
}

// checker implements the errcheck algorithm
type checker struct {
	prog    *loader.Program
	pkg     *loader.PackageInfo
	ignore  map[string]*regexp.Regexp
	blank   bool
	asserts bool
	lines   map[string][]string

	errors []error
}

type uncheckedError struct {
	pos  token.Position
	line string
}

func (e uncheckedError) Error() string {
	pos := e.pos.String()
	if i := strings.Index(pos, "/src/"); i != -1 {
		pos = pos[i+len("/src/"):]
	}
	return fmt.Sprintf("%s\t%s", pos, e.line)
}

func (c *checker) ignoreCall(call *ast.CallExpr) bool {
	// Try to get an identifier.
	// Currently only supports simple expressions:
	//     1. f()
	//     2. x.y.f()
	var id *ast.Ident
	switch exp := call.Fun.(type) {
	case (*ast.Ident):
		id = exp
	case (*ast.SelectorExpr):
		id = exp.Sel
	default:
		// eg: *ast.SliceExpr, *ast.IndexExpr
	}

	if id == nil {
		return false
	}

	// If we got an identifier for the function, see if it is ignored

	if re, ok := c.ignore[""]; ok && re.MatchString(id.Name) {
		return true
	}

	if obj := c.pkg.Uses[id]; obj != nil {
		if pkg := obj.Pkg(); pkg != nil {
			if re, ok := c.ignore[pkg.Path()]; ok {
				return re.MatchString(id.Name)
			}
		}
	}

	return false
}

// errorsByArg returns a slice s such that
// len(s) == number of return types of call
// s[i] == true iff return type at position i from left is an error type
func (c *checker) errorsByArg(call *ast.CallExpr) []bool {
	switch t := c.pkg.Types[call].Type.(type) {
	case *types.Named:
		// Single return
		return []bool{isErrorType(t.Obj())}
	case *types.Tuple:
		// Multiple returns
		s := make([]bool, t.Len())
		for i := 0; i < t.Len(); i++ {
			nt, ok := t.At(i).Type().(*types.Named)
			s[i] = ok && isErrorType(nt.Obj())
		}
		return s
	}
	return []bool{false}
}

func (c *checker) callReturnsError(call *ast.CallExpr) bool {
	if c.isRecover(call) {
		return true
	}
	for _, isError := range c.errorsByArg(call) {
		if isError {
			return true
		}
	}
	return false
}

// isRecover returns true if the given CallExpr is a call to the built-in recover() function.
func (c *checker) isRecover(call *ast.CallExpr) bool {
	if fun, ok := call.Fun.(*ast.Ident); ok {
		if _, ok := c.pkg.Uses[fun].(*types.Builtin); ok {
			return fun.Name == "recover"
		}
	}
	return false
}

func (c *checker) addErrorAtPosition(position token.Pos) {
	pos := c.prog.Fset.Position(position)
	lines, ok := c.lines[pos.Filename]
	if !ok {
		lines = readfile(pos.Filename)
		c.lines[pos.Filename] = lines
	}

	line := "??"
	if pos.Line-1 < len(lines) {
		line = strings.TrimSpace(lines[pos.Line-1])
	}
	c.errors = append(c.errors, uncheckedError{pos, line})
}

func readfile(filename string) []string {
	var f, err = os.Open(filename)
	if err != nil {
		return nil
	}

	var lines []string
	var scanner = bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}

func (c *checker) Visit(node ast.Node) ast.Visitor {
	switch stmt := node.(type) {
	case *ast.ExprStmt:
		if call, ok := stmt.X.(*ast.CallExpr); ok {
			if !c.ignoreCall(call) && c.callReturnsError(call) {
				c.addErrorAtPosition(call.Lparen)
			}
		}
	case *ast.GoStmt:
		if !c.ignoreCall(stmt.Call) && c.callReturnsError(stmt.Call) {
			c.addErrorAtPosition(stmt.Call.Lparen)
		}
	case *ast.DeferStmt:
		if !c.ignoreCall(stmt.Call) && c.callReturnsError(stmt.Call) {
			c.addErrorAtPosition(stmt.Call.Lparen)
		}
	case *ast.AssignStmt:
		if len(stmt.Rhs) == 1 {
			// single value on rhs; check against lhs identifiers
			if call, ok := stmt.Rhs[0].(*ast.CallExpr); ok {
				if !c.blank {
					break
				}
				if c.ignoreCall(call) {
					break
				}
				isError := c.errorsByArg(call)
				for i := 0; i < len(stmt.Lhs); i++ {
					if id, ok := stmt.Lhs[i].(*ast.Ident); ok {
						// We shortcut calls to recover() because errorsByArg can't
						// check its return types for errors since it returns interface{}.
						if id.Name == "_" && (c.isRecover(call) || isError[i]) {
							c.addErrorAtPosition(id.NamePos)
						}
					}
				}
			} else if assert, ok := stmt.Rhs[0].(*ast.TypeAssertExpr); ok {
				if !c.asserts {
					break
				}
				if assert.Type == nil {
					// type switch
					break
				}
				if len(stmt.Lhs) < 2 {
					// assertion result not read
					c.addErrorAtPosition(stmt.Rhs[0].Pos())
				} else if id, ok := stmt.Lhs[1].(*ast.Ident); ok && c.blank && id.Name == "_" {
					// assertion result ignored
					c.addErrorAtPosition(id.NamePos)
				}
			}
		} else {
			// multiple value on rhs; in this case a call can't return
			// multiple values. Assume len(stmt.Lhs) == len(stmt.Rhs)
			for i := 0; i < len(stmt.Lhs); i++ {
				if id, ok := stmt.Lhs[i].(*ast.Ident); ok {
					if call, ok := stmt.Rhs[i].(*ast.CallExpr); ok {
						if !c.blank {
							continue
						}
						if c.ignoreCall(call) {
							continue
						}
						if id.Name == "_" && c.callReturnsError(call) {
							c.addErrorAtPosition(id.NamePos)
						}
					} else if assert, ok := stmt.Rhs[i].(*ast.TypeAssertExpr); ok {
						if !c.asserts {
							continue
						}
						if assert.Type == nil {
							// Shouldn't happen anyway, no multi assignment in type switches
							continue
						}
						c.addErrorAtPosition(id.NamePos)
					}
				}
			}
		}
	default:
	}
	return c
}

type obj interface {
	Pkg() *types.Package
	Name() string
}

func isErrorType(v obj) bool {
	return v.Pkg() == nil && v.Name() == "error"
}
