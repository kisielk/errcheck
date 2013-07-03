// Package errcheck is the library used to implement the errcheck command-line tool.
//
// Note: The API of this package has not been finalized and may change at any point.
package errcheck

import (
	"bytes"
	"code.google.com/p/go.tools/go/types"
	"code.google.com/p/go.tools/importer"
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"io/ioutil"
	"regexp"
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
	return fmt.Sprint(len(e.Errors), "unchecked errors")
}

func CheckPackage(pkgPath string, ignore map[string]*regexp.Regexp, blank bool) error {
	pkg, err := newPackage(pkgPath)
	if err != nil {
		return err
	}

	return checkPackage(pkg, ignore, blank)
}

// package_ represents a single Go package
type package_ struct {
	path  string
	files map[string]file
}

// newPackage creates a package_ from the Go files in path
func newPackage(path string) (package_, error) {
	p := package_{path: path}
	pkg, err := findPackage(path)
	if err != nil {
		return p, fmt.Errorf("could not find package: %s", err)
	}
	fileNames := getFiles(pkg)

	if len(fileNames) == 0 {
		return p, ErrNoGoFiles
	}

	p.files = make(map[string]file, len(fileNames))

	for _, fileName := range fileNames {
		f, err := readFile(fileName)
		if err != nil {
			return p, fmt.Errorf("could not read %s: %s", fileName, err)
		}
		p.files[fileName] = f
	}

	return p, nil
}

// typedPackage is like package_ but with type information
type typedPackage struct {
	package_
	info     *importer.PackageInfo
	importer *importer.Importer
}

// typeCheck creates a typedPackage from a package_
func typeCheck(p package_) (typedPackage, error) {
	context := types.Context{}

	loader := importer.MakeGoBuildLoader(nil)
	importerContext := &importer.Context{
		TypeChecker: context,
		Loader:      loader,
	}
	imp := importer.New(importerContext)
	info, err := imp.LoadPackage(p.path)
	return typedPackage{
		package_: p,
		info:     info,
		importer: imp,
	}, err
}

// file represents a single Go source file
type file struct {
	name  string
	lines [][]byte
}

func readFile(fileName string) (f file, err error) {
	data, err := ioutil.ReadFile(fileName)
	if err != nil {
		return f, err
	}

	lines := bytes.Split(data, []byte("\n"))
	f = file{name: fileName, lines: lines}
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

	if obj := c.pkg.info.ObjectOf(id); obj != nil {
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
	switch t := c.pkg.info.TypeOf(call).(type) {
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
	for _, isError := range c.errorsByArg(call) {
		if isError {
			return true
		}
	}
	return false
}

func (c *checker) addErrorAtPosition(position token.Pos) {
	pos := c.pkg.importer.Fset.Position(position)
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
						if id.Name == "_" && isError[i] {
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
	for _, astFile := range tp.info.Files {
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
