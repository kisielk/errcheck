package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"go/types"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// allImports is a map of already-imported import paths to packages
	allImports map[string]*types.Package

	// ErrCheckErrors is returned by the checkFiles function if any errors were
	// encountered during checking.
	ErrCheckErrors = errors.New("found errors in checked files")
)

// Err prints an error to Stderr
func Err(s string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "error: "+s+"\n", args...)
}

// Fatal calls Err followed by os.Exit(2)
func Fatal(s string, args ...interface{}) {
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
	if f.items == nil {
		f.items = make(map[string]bool)
	}
	for _, item := range strings.Split(s, ",") {
		f.items[item] = true
	}
	return nil
}

func main() {
	allImports = make(map[string]*types.Package)

	var ignore regexpFlag
	flag.Var(&ignore, "ignore", "regular expression of function names to ignore")
	ignorePkg := &stringsFlag{}
	ignorePkg.Set("fmt")
	flag.Var(ignorePkg, "ignorepkg", "comma-separated list of package paths to ignore")
	flag.Parse()

	pkgName := flag.Arg(0)
	if pkgName == "" {
		flag.Usage()
		Fatal("you must specify a package")
	}
	pkg, err := findPackage(pkgName)
	if err != nil {
		Fatal("%s", err)
	}
	files := getFiles(pkg)

	if err := checkFiles(files, ignore.re, ignorePkg.items); err != nil {
		if err == ErrCheckErrors {
			os.Exit(1)
		}
		Fatal("failed to check package: %s", err)
	}
}

// findPackage finds a package.
// path is first tried as an import path and if the package is not found, as a filesystem path.
func findPackage(path string) (*build.Package, error) {
	var (
		err1, err2 error
		pkg        *build.Package
	)

	ctx := build.Default
	ctx.CgoEnabled = false

	// First try to treat path as import path...
	pkg, err1 = ctx.Import(path, ".", 0)
	if err1 != nil {
		// ... then attempt as file path
		pkg, err2 = ctx.ImportDir(path, 0)
	}

	if err2 != nil {
		// Print both errors so the user can see in what ways the
		// package lookup failed.
		return nil, fmt.Errorf("could not import %s: %s\n%s", path, err1, err2)
	}

	return pkg, nil
}

// getFiles returns all the Go files found in a package
func getFiles(pkg *build.Package) []string {
	files := make([]string, len(pkg.GoFiles))
	for i, fileName := range pkg.GoFiles {
		files[i] = filepath.Join(pkg.Dir, fileName)
	}
	return files
}

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

func typeCheck(fset *token.FileSet, astFiles []*ast.File) (map[*ast.CallExpr]types.Type, map[*ast.Ident]types.Object, error) {
	callTypes := make(map[*ast.CallExpr]types.Type)
	identObjs := make(map[*ast.Ident]types.Object)

	exprFn := func(x ast.Expr, typ types.Type, val interface{}) {
		call, ok := x.(*ast.CallExpr)
		if !ok {
			return
		}
		callTypes[call] = typ
	}
	identFn := func(id *ast.Ident, obj types.Object) {
		identObjs[id] = obj
	}
	context := types.Context{
		Expr:   exprFn,
		Ident:  identFn,
		Import: importer,
	}
	_, err := context.Check(fset, astFiles)
	return callTypes, identObjs, err
}

type checker struct {
	fset      *token.FileSet
	files     map[string]file
	callTypes map[*ast.CallExpr]types.Type
	identObjs map[*ast.Ident]types.Object
	ignore    *regexp.Regexp
	ignorePkg map[string]bool

	errors []error
}

type uncheckedErr struct {
	pos  token.Position
	line []byte
}

func (e uncheckedErr) Error() string {
	return fmt.Sprintf("%s %s", e.pos, e.line)
}

func (c *checker) Visit(node ast.Node) ast.Visitor {
	n, ok := node.(*ast.ExprStmt)
	if !ok {
		return c
	}

	// Check for a call expression
	call, ok := n.X.(*ast.CallExpr)
	if !ok {
		return c
	}

	var id *ast.Ident
	switch exp := call.Fun.(type) {
	case (*ast.Ident):
		id = exp
	case (*ast.SelectorExpr):
		id = exp.Sel
	default:
		fmt.Fprintf(os.Stderr, "unhandled expression at %s: %T %+v\n", c.fset.Position(call.Lparen), exp, exp)
		return c
	}

	// Ignore if in an ignored package
	if obj := c.identObjs[id]; obj != nil {
		if pkg := obj.GetPkg(); pkg != nil && c.ignorePkg[pkg.Path] {
			return c
		}
	}
	callType := c.callTypes[call]

	// Ignore if a name matches the regexp
	if c.ignore != nil && c.ignore.MatchString(id.Name) {
		return c
	}

	unchecked := false
	switch t := callType.(type) {
	case *types.NamedType:
		// Single return
		if isErrorType(t.Obj) {
			unchecked = true
		}
	case *types.Result:
		// Multiple returns
		for _, v := range t.Values {
			nt, ok := v.Type.(*types.NamedType)
			if !ok {
				continue
			}
			if isErrorType(nt.Obj) {
				unchecked = true
				break
			}
		}
	}

	if unchecked {
		pos := c.fset.Position(id.NamePos)
		c.errors = append(c.errors, uncheckedErr{pos, c.files[pos.Filename].lines[pos.Line-1]})
	}
	return c
}

func checkFiles(fileNames []string, ignore *regexp.Regexp, ignorePkg map[string]bool) error {
	fset := token.NewFileSet()
	astFiles := make([]*ast.File, len(fileNames))
	files := make(map[string]file, len(fileNames))

	for i, fileName := range fileNames {
		f, err := parseFile(fset, fileName)
		if err != nil {
			return fmt.Errorf("could not parse %s: %s", fileName, err)
		}
		files[fileName] = f
		astFiles[i] = f.ast
	}

	callTypes, identObjs, err := typeCheck(fset, astFiles)
	if err != nil {
		return fmt.Errorf("could not type check: %s", err)
	}

	visitor := &checker{fset, files, callTypes, identObjs, ignore, ignorePkg, []error{}}
	for _, astFile := range astFiles {
		ast.Walk(visitor, astFile)
	}

	for _, e := range visitor.errors {
		fmt.Println(e)
	}

	if len(visitor.errors) > 0 {
		return ErrCheckErrors
	}
	return nil
}

type obj interface {
	GetPkg() *types.Package
	GetName() string
}

func isErrorType(v obj) bool {
	return v.GetPkg() == nil && v.GetName() == "error"
}

func importer(imports map[string]*types.Package, path string) (pkg *types.Package, err error) {
	// types.Importer does not seem to be designed for recursive
	// parsing like we're doing here. Specifically, each nested import
	// will maintain its own imports map. This will lead to duplicate
	// imports and in turn packages, which will lead to funny errors
	// such as "cannot pass argument ip (variable of type net.IP) to
	// variable of type net.IP"
	//
	// To work around this, we keep a global imports map, allImports,
	// to which we add all nested imports, and which we use as the
	// cache, instead of imports.
	//
	// Since all nested imports will also use this importer, there
	// should be no way to end up with duplicate imports.

	// We first try to use GcImport directly. This has the downside of
	// using possibly out-of-date packages, but it has the upside of
	// not having to parse most of the Go standard library.

	buildPkg, buildErr := build.Import(path, ".", 0)

	// If we found no build dir, assume we're dealing with installed
	// but no source. If we found a build dir, only use GcImport if
	// it's in GOROOT. This way we always use up-to-date code for
	// normal packages but avoid parsing the standard library.
	if (buildErr == nil && buildPkg.Goroot) || buildErr != nil {
		pkg, err = types.GcImport(allImports, path)
		if err == nil {
			// We don't use imports, but per API we have to add the package.
			imports[pkg.Path] = pkg
			allImports[pkg.Path] = pkg
			return pkg, nil
		}
	}

	// See if we already imported this package
	if pkg = allImports[path]; pkg != nil && pkg.Complete {
		return pkg, nil
	}

	// allImports failed, try to use go/build
	if buildErr != nil {
		return nil, buildErr
	}

	fileSet := token.NewFileSet()

	isGoFile := func(d os.FileInfo) bool {
		allFiles := make([]string, 0, len(buildPkg.GoFiles)+len(buildPkg.CgoFiles))
		allFiles = append(allFiles, buildPkg.GoFiles...)
		allFiles = append(allFiles, buildPkg.CgoFiles...)

		for _, file := range allFiles {
			if file == d.Name() {
				return true
			}
		}
		return false
	}
	pkgs, err := parser.ParseDir(fileSet, buildPkg.Dir, isGoFile, 0)
	if err != nil {
		return nil, err
	}

	delete(pkgs, "documentation")
	var astPkg *ast.Package
	var name string
	for name, astPkg = range pkgs {
		// Use the first non-main package, or the only package we
		// found.
		//
		// NOTE(dh) I can't think of a reason why there should be
		// multiple packages in a single directory, but ParseDir
		// accommodates for that possibility.
		if len(pkgs) == 1 || name != "main" {
			break
		}
	}

	if astPkg == nil {
		return nil, fmt.Errorf("can't find import: %s", name)
	}

	var ff []*ast.File
	for _, f := range astPkg.Files {
		ff = append(ff, f)
	}

	context := types.Context{
		Import: importer,
	}

	pkg, err = context.Check(fileSet, ff)
	if err != nil {
		return pkg, err
	}

	// We don't use imports, but per API we have to add the package.
	imports[path] = pkg
	allImports[path] = pkg
	pkg.Complete = true
	return pkg, nil
}
