package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
)

func Err(s string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "error:"+s+"\n", args...)
}

func main() {
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

	for _, fileName := range pkg.GoFiles {
		filePath := filepath.Join(pkg.Dir, fileName)
		if err := checkFile(filePath); err != nil {
			Err("could not check %s: %s", filePath, err)
		}
	}
}

func checkFile(fileName string) error {
	fset := token.NewFileSet()

	astFile, err := parser.ParseFile(fset, fileName, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("could not parse: %s", err)
	}

	callTypes := make(map[*ast.CallExpr]types.Type)

	exprFn := func(x ast.Expr, typ types.Type, val interface{}) {
		call, ok := x.(*ast.CallExpr)
		if !ok {
			return
		}
		callTypes[call] = typ
	}
	context := types.Context{
		Expr: exprFn,
	}
	_, err = context.Check(fset, []*ast.File{astFile})
	if err != nil {
		return err
	}

	visitor := func(node ast.Node) {
		n, ok := node.(*ast.ExprStmt)
		if !ok {
			return
		}

		// Check for a call expression
		call, ok := n.X.(*ast.CallExpr)
		if !ok {
			return
		}

		var fun *ast.Ident
		switch exp := call.Fun.(type) {
		case (*ast.Ident):
			fun = exp
		case (*ast.SelectorExpr):
			fun = exp.Sel
		default:
			fmt.Fprintf(os.Stderr, "unknown call: %T %+v\n", exp, exp)
			return
		}

		// Get the types
		callType := callTypes[call]

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
			fmt.Fprintf(os.Stdout, "%s\n", fset.Position(fun.NamePos))
		}
	}

	ast.Walk(visitorFunc(visitor), astFile)
	//	ast.Fprint(os.Stderr, fset, astFile, nil)

	return nil
}

type obj interface {
	GetPkg() *types.Package
	GetName() string
}

func isErrorType(v obj) bool {
	return v.GetPkg() == nil && v.GetName() == "error"
}

type visitorFunc func(node ast.Node)

func (v visitorFunc) Visit(node ast.Node) ast.Visitor {
	v(node)
	return v
}
