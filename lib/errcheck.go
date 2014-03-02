// Package errcheck is the library used to implement the errcheck command-line tool.
//
// Note: The API of this package has not been finalized and may change at any point.
package errcheck

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"regexp"

	"code.google.com/p/go.tools/go/types"
	"honnef.co/go/importer"
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

// CheckPackage checks a package at pkgPath for errors.
// ignore is a map of package names to regular expressions. Identifiers from a package are
// checked against its regular expressions and if any of the expressions match the call
// is not checked.
// If blank is true then assignments to the blank identifier are also considered to be
// ignored errors.
func CheckPackage(pkgPath string, ignore map[string]*regexp.Regexp, blank bool) error {
	pkg, err := newPackage(pkgPath)
	if err != nil {
		return err
	}

	return checkPackage(pkg, ignore, blank)
}

// package_ represents a single Go package
type package_ struct {
	path     string
	fset     *token.FileSet
	astFiles []*ast.File
	files    map[string]file
}

// newPackage creates a package_ from the Go files in path
func newPackage(path string) (package_, error) {
	p := package_{path: path, fset: token.NewFileSet()}
	pkg, err := findPackage(path)
	if err != nil {
		return p, fmt.Errorf("could not find package: %s", err)
	}
	fileNames := getFiles(pkg)

	if len(fileNames) == 0 {
		return p, ErrNoGoFiles
	}

	p.astFiles = make([]*ast.File, len(fileNames))
	p.files = make(map[string]file, len(fileNames))

	for i, fileName := range fileNames {
		f, err := parseFile(p.fset, fileName)
		if err != nil {
			return p, fmt.Errorf("could not parse %s: %s", fileName, err)
		}
		p.files[fileName] = f
		p.astFiles[i] = f.ast
	}

	return p, nil
}

// typedPackage is like package_ but with type information
type typedPackage struct {
	package_
	callTypes map[ast.Expr]types.TypeAndValue
	defs      map[*ast.Ident]types.Object
	uses      map[*ast.Ident]types.Object
}

// typeCheck creates a typedPackage from a package_
func typeCheck(p package_) (typedPackage, error) {
	tp := typedPackage{
		package_:  p,
		callTypes: make(map[ast.Expr]types.TypeAndValue),
		defs:      make(map[*ast.Ident]types.Object),
		uses:      make(map[*ast.Ident]types.Object),
	}

	info := types.Info{
		Types: tp.callTypes,
		Defs:  tp.defs,
		Uses:  tp.uses,
	}
	imp := importer.New()
	// Preliminary cgo support.
	// https://github.com/kisielk/errcheck/issues/16#issuecomment-26436917
	imp.Config = importer.Config{UseGcFallback: true}
	context := types.Config{Import: imp.Import}

	_, err := context.Check(p.path, p.fset, p.astFiles, &info)
	return tp, err
}

// file represents a single Go source file
type file struct {
	fset  *token.FileSet
	name  string
	ast   *ast.File
	lines [][]byte
}

func parseFile(fset *token.FileSet, fileName string) (f file, err error) {
	rd, err := os.Open(fileName)
	if err != nil {
		return f, err
	}
	defer rd.Close()

	data, err := ioutil.ReadAll(rd)
	if err != nil {
		return f, err
	}

	astFile, err := parser.ParseFile(fset, fileName, bytes.NewReader(data), parser.ParseComments)
	if err != nil {
		return f, fmt.Errorf("could not parse: %s", err)
	}

	lines := bytes.Split(data, []byte("\n"))
	f = file{fset: fset, name: fileName, ast: astFile, lines: lines}
	return f, nil
}

// checker implements the errcheck algorithm
type checker struct {
	pkg    typedPackage
	ignore map[string]*regexp.Regexp
	blank  bool

	errors []error
}

type uncheckedError struct {
	pos  token.Position
	line []byte
}

func (e uncheckedError) Error() string {
	return fmt.Sprintf("%s\t%s", e.pos, e.line)
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

	if obj := c.pkg.uses[id]; obj != nil {
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
	switch t := c.pkg.callTypes[call].Type.(type) {
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
	return nil
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
		if _, ok := c.pkg.uses[fun].(*types.Builtin); ok {
			return fun.Name == "recover"
		}
	}
	return false
}

func (c *checker) addErrorAtPosition(position token.Pos) {
	pos := c.pkg.fset.Position(position)
	line := bytes.TrimSpace(c.pkg.files[pos.Filename].lines[pos.Line-1])
	c.errors = append(c.errors, uncheckedError{pos, line})
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
		if !c.blank {
			break
		}
		if len(stmt.Rhs) == 1 {
			// single value on rhs; check against lhs identifiers
			if call, ok := stmt.Rhs[0].(*ast.CallExpr); ok {
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
			}
		} else {
			// multiple value on rhs; in this case a call can't return
			// multiple values. Assume len(stmt.Lhs) == len(stmt.Rhs)
			for i := 0; i < len(stmt.Lhs); i++ {
				if id, ok := stmt.Lhs[i].(*ast.Ident); ok {
					if call, ok := stmt.Rhs[i].(*ast.CallExpr); ok {
						if c.ignoreCall(call) {
							continue
						}
						if id.Name == "_" && c.callReturnsError(call) {
							c.addErrorAtPosition(id.NamePos)
						}
					}
				}
			}
		}
	default:
	}
	return c
}

func checkPackage(pkg package_, ignore map[string]*regexp.Regexp, blank bool) error {
	tp, err := typeCheck(pkg)
	if err != nil {
		return fmt.Errorf("could not type check: %s", err)
	}

	visitor := &checker{tp, ignore, blank, []error{}}
	for _, astFile := range pkg.astFiles {
		ast.Walk(visitor, astFile)
	}

	if len(visitor.errors) > 0 {
		return UncheckedErrors{visitor.errors}
	}
	return nil
}

type obj interface {
	Pkg() *types.Package
	Name() string
}

func isErrorType(v obj) bool {
	return v.Pkg() == nil && v.Name() == "error"
}
