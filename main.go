package main

import (
	"bytes"
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

var allImports map[string]*types.Package

func Err(s string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "error:"+s+"\n", args...)
}

func main() {
	allImports = make(map[string]*types.Package)

	ignore := flag.String("ignore", "", "regular expression of function names to ignore")
	ignorePkg := flag.String("ignorepkg", "fmt", "comma-separated list of package paths to ignore")
	flag.Parse()
	pkgName := flag.Arg(0)
	if pkgName == "" {
		Err("you must specify a package")
		flag.Usage()
		os.Exit(1)
	}

	pkg, err := build.Import(pkgName, ".", 0)
	if err != nil {
		Err("could not import %s: %s", pkgName, err)
		os.Exit(1)
	}
	files := make([]string, len(pkg.GoFiles))
	for i, fileName := range pkg.GoFiles {
		files[i] = filepath.Join(pkg.Dir, fileName)
	}

	if err := checkFiles(files, *ignore, strings.Split(*ignorePkg, ",")); err != nil {
		Err("failed to check package: %s", err)
		os.Exit(1)
	}
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
}

func (c checker) Visit(node ast.Node) ast.Visitor {
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
		fmt.Fprintf(os.Stderr, "unknown call: %T %+v\n", exp, exp)
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
		fmt.Fprintf(os.Stdout, "%s %s\n", pos, c.files[pos.Filename].lines[pos.Line-1])
	}
	return c
}

func checkFiles(fileNames []string, ignore string, ignorePkg []string) error {
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

	ignorePkgSet := make(map[string]bool)
	for _, pkg := range ignorePkg {
		ignorePkgSet[pkg] = true
	}

	var ignoreRe *regexp.Regexp
	if ignore != "" {
		var err error
		ignoreRe, err = regexp.Compile(ignore)
		if err != nil {
			return fmt.Errorf("invalid ignore regexp: %s", err)
		}
	}

	visitor := checker{fset, files, callTypes, identObjs, ignoreRe, ignorePkgSet}
	for _, astFile := range astFiles {
		ast.Walk(visitor, astFile)
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
