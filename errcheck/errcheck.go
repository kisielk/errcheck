// Package errcheck is the library used to implement the errcheck command-line tool.
package errcheck

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"
)

// errorType is the type of the error interface.
// It's initialized when the package is imported to avoid lookup on every call to isErrorType.
var errorType = types.Universe.Lookup("error").Type().Underlying().(*types.Interface)

// isErrorType reports whether t is an error type.
func isErrorType(t types.Type) bool {
	if t == nil {
		return false
	}
	return types.Implements(types.Unalias(t), errorType)
}

// UncheckedError indicates the position of an unchecked error return.
type UncheckedError struct {
	Pos          token.Position
	Line         string
	FuncName     string
	SelectorName string
}

// Result is returned from the CheckPackage function, and holds all the errors
// that were found to be unchecked in a package.
//
// Aggregation can be done using the Append method for users that want to
// combine results from multiple packages.
type Result struct {
	// UncheckedErrors is a list of all the unchecked errors in the package.
	// Printing an error reports its position within the file and the contents of the line.
	UncheckedErrors []UncheckedError
}

// compareUncheckedErrors orders errors by position, then by the remaining fields.
// It compares every field so that errors which are == always sort next to each
// other, which Unique relies on to remove duplicates.
func compareUncheckedErrors(a, b UncheckedError) int {
	return cmp.Or(
		strings.Compare(a.Pos.Filename, b.Pos.Filename),
		cmp.Compare(a.Pos.Line, b.Pos.Line),
		cmp.Compare(a.Pos.Column, b.Pos.Column),
		cmp.Compare(a.Pos.Offset, b.Pos.Offset),
		strings.Compare(a.Line, b.Line),
		strings.Compare(a.FuncName, b.FuncName),
		strings.Compare(a.SelectorName, b.SelectorName),
	)
}

// Append appends errors to e. Append does not do any duplicate checking.
func (r *Result) Append(other Result) {
	r.UncheckedErrors = append(r.UncheckedErrors, other.UncheckedErrors...)
}

// Unique returns the unique errors that have been accumulated. Duplicates may occur
// when a file containing an unchecked error belongs to > 1 package.
//
// The method receiver remains unmodified after the call to Unique.
func (r *Result) Unique() Result {
	result := slices.Clone(r.UncheckedErrors)
	slices.SortFunc(result, compareUncheckedErrors)
	return Result{UncheckedErrors: slices.Compact(result)}
}

// Exclusions define symbols and language elements that will be not checked
type Exclusions struct {

	// Packages lists paths of excluded packages.
	Packages []string

	// SymbolRegexpsByPackage maps individual package paths to regular
	// expressions that match symbols to be excluded.
	//
	// Packages whose paths appear both here and in Packages list will
	// be excluded entirely.
	//
	// This is a legacy input that will be deprecated in errcheck version 2 and
	// should not be used.
	SymbolRegexpsByPackage map[string]*regexp.Regexp

	// Symbols lists patterns that exclude individual package symbols.
	//
	// For example:
	//
	//   "fmt.Errorf"              // function
	//   "fmt.Fprintf(os.Stderr)"  // function with set argument value
	//   "(hash.Hash).Write"       // method
	//
	Symbols []string

	// TestFiles excludes _test.go files.
	TestFiles bool

	// GeneratedFiles excludes generated source files.
	//
	// Source file is assumed to be generated if its contents
	// match the following regular expression:
	//
	//   ^// Code generated .* DO NOT EDIT\\.$
	//
	GeneratedFiles bool

	// BlankAssignments ignores assignments to blank identifier.
	BlankAssignments bool

	// TypeAssertions ignores unchecked type assertions.
	TypeAssertions bool
}

// Checker checks that you checked errors.
type Checker struct {
	// Exclusions defines code packages, symbols, and other elements that will not be checked.
	Exclusions Exclusions

	// Tags are a list of build tags to use.
	Tags []string

	// The mod flag for go build.
	Mod string
}

// loadPackages is used for testing.
var loadPackages = func(cfg *packages.Config, paths ...string) ([]*packages.Package, error) {
	return packages.Load(cfg, paths...)
}

// LoadPackages loads all the packages in all the paths provided. It uses the
// exclusions and build tags provided to by the user when loading the packages.
func (c *Checker) LoadPackages(paths ...string) ([]*packages.Package, error) {
	buildFlags := []string{fmt.Sprintf("-tags=%s", strings.Join(c.Tags, ","))}
	if c.Mod != "" {
		buildFlags = append(buildFlags, fmt.Sprintf("-mod=%s", c.Mod))
	}
	cfg := &packages.Config{
		Mode:       packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Tests:      !c.Exclusions.TestFiles,
		BuildFlags: buildFlags,
	}
	return loadPackages(cfg, paths...)
}

var generatedCodeRegexp = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)
var dotStar = regexp.MustCompile(".*")

func (c *Checker) shouldSkipFile(file *ast.File) bool {
	if !c.Exclusions.GeneratedFiles {
		return false
	}

	for _, cg := range file.Comments {
		for _, comment := range cg.List {
			if generatedCodeRegexp.MatchString(comment.Text) {
				return true
			}
		}
	}

	return false
}

// CheckPackage checks packages for errors that have not been checked.
//
// It will exclude specific errors from analysis if the user has configured
// exclusions.
func (c *Checker) CheckPackage(pkg *packages.Package) Result {
	excludedSymbols := map[string]bool{}
	for _, sym := range c.Exclusions.Symbols {
		excludedSymbols[sym] = true
	}

	ignore := map[string]*regexp.Regexp{}
	// Apply SymbolRegexpsByPackage first so that if the same path appears in
	// Packages, a more narrow regexp will be superseded by dotStar below.
	for pkg, re := range c.Exclusions.SymbolRegexpsByPackage {
		ignore[nonVendoredPkgPath(pkg)] = re
	}
	for _, pkg := range c.Exclusions.Packages {
		ignore[nonVendoredPkgPath(pkg)] = dotStar
	}

	v := &visitor{
		typesInfo:             pkg.TypesInfo,
		fset:                  pkg.Fset,
		ignore:                ignore,
		checkBlankAssignments: !c.Exclusions.BlankAssignments,
		checkTypeAsserts:      !c.Exclusions.TypeAssertions,
		lines:                 make(map[string][]string),
		exclude:               excludedSymbols,
		errors:                []UncheckedError{},
	}

	for _, astFile := range pkg.Syntax {
		if c.shouldSkipFile(astFile) {
			continue
		}
		ast.Walk(v, astFile)
	}
	return Result{UncheckedErrors: v.errors}
}

// visitor implements the errcheck algorithm
type visitor struct {
	typesInfo             *types.Info
	fset                  *token.FileSet
	ignore                map[string]*regexp.Regexp
	checkBlankAssignments bool
	checkTypeAsserts      bool
	lines                 map[string][]string
	exclude               map[string]bool

	// Type-switch narrowing state (used only when checkTypeAsserts is true).
	//
	// currentFn is the immediately enclosing *ast.FuncDecl or *ast.FuncLit,
	// and closureMutated lazily records variables modified inside nested
	// function literals within currentFn.
	//
	// narrowed maps a local variable x to its active enclosing type-switch
	// case type lists. Each outer slice element represents one enclosing
	// CaseClause on x, and its inner slice holds the types listed in that
	// case. For example, in:
	//
	//   switch x.(type) {
	//   case io.ReadCloser, *bytes.Buffer: // inner slice: [io.ReadCloser, *bytes.Buffer]
	//       switch x.(type) {
	//       case *bytes.Buffer:            // inner slice: [*bytes.Buffer]
	//       }
	//   }
	//
	// narrowed[x] holds [[io.ReadCloser, *bytes.Buffer], [*bytes.Buffer]].
	currentFn        ast.Node
	closureInfoReady bool
	closureMutated   map[*types.Var]bool
	narrowed         map[*types.Var][][]types.Type

	errors []UncheckedError
}

// selectorAndFunc tries to get the selector and function from call expression.
// For example, given the call expression representing "a.b()", the selector
// is "a.b" and the function is "b" itself.
//
// The final return value will be true if it is able to do extract a selector
// from the call and look up the function object it refers to.
//
// If the call does not include a selector (like if it is a plain "f()" function call)
// then the final return value will be false.
func (v *visitor) selectorAndFunc(call *ast.CallExpr) (*ast.SelectorExpr, *types.Func, bool) {
	sel, ok := baseCallExpr(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return nil, nil, false
	}

	fn, ok := v.typesInfo.ObjectOf(sel.Sel).(*types.Func)
	if !ok {
		// Shouldn't happen, but be paranoid
		return nil, nil, false
	}

	return sel, fn, true

}

// getSelectorName returns a name for a called function
// if the function is the result of a selector. Otherwise, it returns
// an empty string.
//
// The name is fully qualified by the import path, possible type,
// function/method name and pointer receiver.
//
// For example:
//   - "fmt.Printf(...)" -> "fmt.Printf"
//   - "base64.StdEncoding.Decode(...)" -> "base64.StdEncoding.Decode"
//   - "myFunc()" -> ""
func getSelectorName(sel *ast.SelectorExpr) string {
	if ident, ok := sel.X.(*ast.Ident); ok {
		return ident.Name + "." + sel.Sel.Name
	}
	if s, ok := sel.X.(*ast.SelectorExpr); ok {
		return getSelectorName(s) + "." + sel.Sel.Name
	}

	return ""
}

// namesForExcludeCheck returns the names to look up in the exclusion list
// for a call of fn through the selector sel, as returned by selectorAndFunc.
// sel is nil for a call by name alone, like "F()".
//
// For a package-level function or a method of a concrete type, this is just
// fn's full name, such as "fmt.Printf" or "(*bytes.Buffer).Write".
//
// For a method declared in an interface, it returns a name for each interface
// between the receiver and the interface that declares the method, looking
// through embedded struct fields and embedded interfaces. For example, Write
// called on an io.ReadWriter gives "(io.ReadWriter).Write" and
// "(io.Writer).Write", so excluding "(io.Writer).Write" also excludes calls
// through interfaces that embed io.Writer.
func (v *visitor) namesForExcludeCheck(sel *ast.SelectorExpr, fn *types.Func) []string {
	name := fn.FullName()

	// This will be missing for functions without a receiver (like fmt.Printf),
	// so just fall back to the function's full name in that case.
	selection, ok := v.typesInfo.Selections[sel]
	if !ok {
		return []string{name}
	}

	// This will return with ok false if the function isn't defined
	// on an interface, so just fall back to the full name.
	ts, ok := walkThroughEmbeddedInterfaces(selection)
	if !ok {
		return []string{name}
	}

	result := make([]string, len(ts))
	for i, t := range ts {
		result[i] = fmt.Sprintf("(%s).%s", t.String(), fn.Name())
	}
	return result
}

// argTypeName returns the name of the type of the given expression.
// As a special case, if the expression is the literal, "os.Stdout" or "os.Stderr"
// then those strings are returned.
// It returns "" if expr has no type.
// This function is primarily used to match against exclusion patterns.
func (v *visitor) argTypeName(expr ast.Expr) string {
	// Special-case literal "os.Stdout" and "os.Stderr", even in parentheses.
	if sel, ok := ast.Unparen(expr).(*ast.SelectorExpr); ok {
		if obj := v.typesInfo.ObjectOf(sel.Sel); obj != nil {
			vr, ok := obj.(*types.Var)
			if ok && vr.Pkg() != nil && vr.Pkg().Path() == "os" && (vr.Name() == "Stderr" || vr.Name() == "Stdout") {
				return "os." + vr.Name()
			}
		}
	}
	t := v.typesInfo.TypeOf(expr)
	if t == nil {
		return ""
	}
	return t.String()
}

// excludeCall reports whether call, a call of fn through the selector sel,
// matches the exclusion list. sel is nil if fn is called by name alone.
//
// A call matches if the list contains one of the names from
// namesForExcludeCheck, either alone or followed by the type of the call's
// first argument, such as "fmt.Fprintf(*bytes.Buffer)".
func (v *visitor) excludeCall(call *ast.CallExpr, sel *ast.SelectorExpr, fn *types.Func) bool {
	var arg0 string
	if len(call.Args) > 0 {
		arg0 = v.argTypeName(call.Args[0])
	}
	for _, name := range v.namesForExcludeCheck(sel, fn) {
		if v.exclude[name] {
			return true
		}
		if arg0 != "" && v.exclude[name+"("+arg0+")"] {
			return true
		}
	}
	return false
}

// ignoreCall reports whether call is excluded from checking, by the
// exclusion list, the ignore regexps, or the errors.AsType special case.
func (v *visitor) ignoreCall(call *ast.CallExpr) bool {
	// The exclusion list and errors.AsType apply to calls of declared
	// functions and methods, through a selector like "pkg.F()" or "x.M()",
	// or by name alone, like "F()" in F's own package or after a dot import.
	// sel is nil for calls by name.
	sel, fn, ok := v.selectorAndFunc(call)
	if !ok {
		if id, isIdent := baseCallExpr(call.Fun).(*ast.Ident); isIdent {
			fn, ok = v.typesInfo.Uses[id].(*types.Func)
		}
	}
	if ok && (v.excludeCall(call, sel, fn) || isErrorsAsType(fn)) {
		return true
	}

	// Try to get an identifier.
	// Currently only supports simple expressions:
	//     1. f()
	//     2. x.y.f()
	//     3. x.y[T]() or x.y[T1, T2]()
	var id *ast.Ident
	switch exp := baseCallExpr(call.Fun).(type) {
	case *ast.Ident:
		id = exp
	case *ast.SelectorExpr:
		id = exp.Sel
	default:
		// eg: *ast.SliceExpr
	}

	if id == nil {
		return false
	}

	// If we got an identifier for the function, see if it is ignored
	if re, ok := v.ignore[""]; ok && re.MatchString(id.Name) {
		return true
	}

	if obj := v.typesInfo.Uses[id]; obj != nil {
		if pkg := obj.Pkg(); pkg != nil {
			if re, ok := v.ignore[nonVendoredPkgPath(pkg.Path())]; ok {
				return re.MatchString(id.Name)
			}
		}
	}

	return false
}

// baseCallExpr returns the underlying function expression for a call. Type
// arguments and parentheses wrap the selector/name in additional AST nodes, so
// matching call.Fun directly would miss forms like errors.AsType[T](err).
func baseCallExpr(fun ast.Expr) ast.Expr {
	for {
		switch f := fun.(type) {
		case *ast.IndexExpr:
			fun = f.X
		case *ast.IndexListExpr:
			fun = f.X
		case *ast.ParenExpr:
			fun = f.X
		default:
			return fun
		}
	}
}

// isErrorsAsType reports whether fn, a called function, is the standard
// library's errors.AsType. It goes by the resolved function, not by how the
// call is spelled, so a renamed or dot import still matches and a local
// variable or other package named "errors" does not.
func isErrorsAsType(fn *types.Func) bool {
	return fn.Name() == "AsType" && fn.Pkg() != nil && fn.Pkg().Path() == "errors"
}

// nonVendoredPkgPath returns the unvendored version of the provided package
// path (or returns the provided path if it does not represent a vendored
// path).
func nonVendoredPkgPath(pkgPath string) string {
	lastVendorIndex := strings.LastIndex(pkgPath, "/vendor/")
	if lastVendorIndex == -1 {
		return pkgPath
	}
	return pkgPath[lastVendorIndex+len("/vendor/"):]
}

// errorsByArg returns a slice s such that
// len(s) == number of return types of call
// s[i] == true iff return type at position i from left is an error type
func (v *visitor) errorsByArg(call *ast.CallExpr) []bool {
	// call.Fun may denote a type rather than a
	// function/method value, e.g. in a type conversion such as
	// (*T)(x). Go's AST represents conversions as CallExpr just like
	// real calls, but a conversion isn't a call and can't itself
	// "return" an error - even when T happens to implement the error
	// interface (as in `var _ error = (*T)(nil)`, a common compile-time
	// interface-satisfaction check).
	// See https://github.com/kisielk/errcheck/issues/230
	if v.typesInfo.Types[call.Fun].IsType() {
		return []bool{false}
	}

	// Any result type can implement error, including type parameters
	// constrained by error and unnamed interface types, so every result
	// is checked with isErrorType (which also looks through aliases).
	t := v.typesInfo.Types[call].Type
	if tuple, ok := t.(*types.Tuple); ok {
		// Multiple returns
		s := make([]bool, tuple.Len())
		for i := range tuple.Len() {
			s[i] = isErrorType(tuple.At(i).Type())
		}
		return s
	}
	return []bool{isErrorType(t)}
}

func (v *visitor) callReturnsError(call *ast.CallExpr) bool {
	if v.isRecover(call) {
		return true
	}
	for _, isError := range v.errorsByArg(call) {
		if isError {
			return true
		}
	}
	return false
}

// isRecover returns true if the given CallExpr is a call to the built-in recover() function.
func (v *visitor) isRecover(call *ast.CallExpr) bool {
	if fun, ok := call.Fun.(*ast.Ident); ok {
		if _, ok := v.typesInfo.Uses[fun].(*types.Builtin); ok {
			return fun.Name == "recover"
		}
	}
	return false
}

// TODO (dtcaciuc) collect token.Pos and then convert them to UncheckedErrors
// after visitor is done running. This will allow to integrate more cleanly
// with analyzer so that we don't have to convert Position back to Pos.
func (v *visitor) addErrorAtPosition(position token.Pos, call *ast.CallExpr) {
	pos := v.fset.Position(position)
	lines, ok := v.lines[pos.Filename]
	if !ok {
		// We haven't read and cached the file yet, so read it now. If it
		// can't be read, cache nil so every line in it shows as "??".
		if data, err := os.ReadFile(pos.Filename); err == nil {
			lines = strings.Split(string(data), "\n")
		}
		v.lines[pos.Filename] = lines
	}

	line := "??"
	if pos.Line-1 < len(lines) {
		line = strings.TrimSpace(lines[pos.Line-1])
	}

	// Both names are only set for selector calls like "fmt.Printf(...)",
	// giving "fmt.Printf" and "fmt.Printf", or "base64.StdEncoding.Decode(...)",
	// giving "(*encoding/base64.Encoding).Decode" and "base64.StdEncoding.Decode".
	var funcName, selectorName string
	if call != nil {
		if sel, fn, ok := v.selectorAndFunc(call); ok {
			funcName = fn.FullName()
			selectorName = getSelectorName(sel)
		}
	}

	v.errors = append(v.errors, UncheckedError{pos, line, funcName, selectorName})
}

func (v *visitor) Visit(node ast.Node) ast.Visitor {
	switch stmt := node.(type) {
	case *ast.ExprStmt:
		if call := unwrapCall(stmt.X); call != nil {
			if v.callReturnsError(call) && !v.ignoreCall(call) {
				v.addErrorAtPosition(call.Lparen, call)
			}
		} else if unary := unwrapUnary(stmt.X); unary != nil && unary.Op == token.ARROW {
			if v.isChannelReceiveReturningError(unary) {
				v.addErrorAtPosition(unary.OpPos, nil)
			}
		}
	case *ast.GoStmt:
		if v.callReturnsError(stmt.Call) && !v.ignoreCall(stmt.Call) {
			v.addErrorAtPosition(stmt.Call.Lparen, stmt.Call)
		}
	case *ast.DeferStmt:
		if v.callReturnsError(stmt.Call) && !v.ignoreCall(stmt.Call) {
			v.addErrorAtPosition(stmt.Call.Lparen, stmt.Call)
		}
	case *ast.ValueSpec:
		// Previously we checked on *ast.GenDecl but that resulted in one
		// check for a whole var or const block, e.g.:
		//    var (
		//        a = b
		//        c = d
		//    )
		// so that if the first assignment was skipped, the rest would be too.
		//
		// Instead, by using ast.ValueSpec, we check each assignment separately.
		lhs := make([]ast.Expr, len(stmt.Names))
		for i, name := range stmt.Names {
			lhs[i] = name
		}
		if !v.checkAssignment(lhs, stmt.Values) {
			return nil
		}

	case *ast.FuncDecl:
		// Scope type-switch narrowing to this function body so outer
		// narrowing cannot leak across function boundaries.
		if v.checkTypeAsserts {
			if stmt.Recv != nil {
				ast.Walk(v, stmt.Recv)
			}
			if stmt.Type != nil {
				ast.Walk(v, stmt.Type)
			}
			if stmt.Body != nil {
				v.walkFuncBody(stmt, stmt.Body)
			}
			return nil
		}

	case *ast.FuncLit:
		// A closure may run after the enclosing case finishes and the
		// switched variable has been reassigned, so start with empty
		// narrowing inside the closure body:
		//
		//   switch x.(type) {
		//   case string:
		//       defer func() { _ = x.(string) }() // unchecked
		//   }
		//   x = 42
		if v.checkTypeAsserts {
			if stmt.Type != nil {
				ast.Walk(v, stmt.Type)
			}
			if stmt.Body != nil {
				v.walkFuncBody(stmt, stmt.Body)
			}
			return nil
		}

	case *ast.TypeSwitchStmt:
		// Track per-case type narrowing on the switched variable:
		//
		//   switch x.(type) {
		//   case string:
		//       _ = x.(string) // guaranteed not to panic
		//   }
		if v.checkTypeAsserts {
			v.walkTypeSwitchStmt(stmt)
			return nil
		}

	case *ast.AssignStmt:
		// Evaluate the assignment's expressions before invalidating LHS
		// variables so assertions on the RHS still see active narrowing,
		// while subsequent statements see that the LHS was overwritten:
		//
		//   switch x.(type) {
		//   case string:
		//       s := x.(string) // safe: x is still string on RHS
		//       x = 42          // invalidates narrowing on x
		//       _ = x.(string)  // unchecked
		//   }
		followed := v.checkAssignment(stmt.Lhs, stmt.Rhs)
		if len(v.narrowed) == 0 {
			if !followed {
				return nil
			}
			return v
		}
		if followed {
			for _, lhs := range stmt.Lhs {
				ast.Walk(v, lhs)
			}
			for _, rhs := range stmt.Rhs {
				ast.Walk(v, rhs)
			}
		}
		for _, lhs := range stmt.Lhs {
			v.invalidateIdent(lhs)
		}
		return nil

	case *ast.IncDecStmt:
		// Walk the operand first, then invalidate narrowing if it is a
		// tracked variable (e.g. x++).
		if len(v.narrowed) > 0 {
			ast.Walk(v, stmt.X)
			v.invalidateIdent(stmt.X)
			return nil
		}

	case *ast.UnaryExpr:
		// Taking the address of a narrowed variable allows indirect
		// mutation of its dynamic type through the pointer:
		//
		//   switch x.(type) {
		//   case string:
		//       setAny(&x)     // may store a non-string into *(&x)
		//       _ = x.(string) // unchecked
		//   }
		if len(v.narrowed) > 0 && stmt.Op == token.AND {
			v.invalidateIdent(stmt.X)
		}

	case *ast.IfStmt:
		// Walk the then and else branches from independent copies of the
		// pre-branch narrowing, then invalidate anything mutated in
		// either branch for code after the if:
		//
		//   switch x.(type) {
		//   case string:
		//       if cond {
		//           x = 42
		//       } else {
		//           _ = x.(string) // safe: x = 42 did not run here
		//       }
		//       _ = x.(string)     // unchecked: x may now be 42
		//   }
		if len(v.narrowed) > 0 {
			if stmt.Init != nil {
				ast.Walk(v, stmt.Init)
			}
			ast.Walk(v, stmt.Cond)
			saved := maps.Clone(v.narrowed)
			ast.Walk(v, stmt.Body)
			if stmt.Else != nil {
				v.narrowed = maps.Clone(saved)
				ast.Walk(v, stmt.Else)
			}
			v.narrowed = saved
			v.invalidateMutatedIn(stmt.Body)
			v.invalidateMutatedIn(stmt.Else)
			return nil
		}

	case *ast.SwitchStmt:
		// Each expression-switch case runs independently unless a case
		// falls through into the next one:
		//
		//   switch x.(type) {
		//   case string:
		//       switch n {
		//       case 1:
		//           x = 42
		//       case 2:
		//           _ = x.(string) // safe (no fallthrough from case 1)
		//       }
		//       _ = x.(string)     // unchecked after switch
		//   }
		if len(v.narrowed) > 0 {
			if stmt.Init != nil {
				ast.Walk(v, stmt.Init)
			}
			if stmt.Tag != nil {
				ast.Walk(v, stmt.Tag)
			}
			saved := maps.Clone(v.narrowed)
			if stmt.Body != nil {
				if containsFallthrough(stmt.Body) {
					v.invalidateMutatedIn(stmt.Body)
					ast.Walk(v, stmt.Body)
				} else {
					for _, s := range stmt.Body.List {
						if cc, ok := s.(*ast.CaseClause); ok {
							v.narrowed = maps.Clone(saved)
							for _, e := range cc.List {
								ast.Walk(v, e)
							}
							for _, bs := range cc.Body {
								ast.Walk(v, bs)
							}
						} else {
							ast.Walk(v, s)
						}
					}
				}
				v.narrowed = saved
				v.invalidateMutatedIn(stmt.Body)
			}
			return nil
		}

	case *ast.SelectStmt:
		// Each select communication clause runs independently, and any
		// variable mutated in any clause is invalidated after the select:
		//
		//   switch x.(type) {
		//   case string:
		//       select {
		//       case x = <-ch:
		//       default:
		//           _ = x.(string) // safe in this clause
		//       }
		//       _ = x.(string)     // unchecked after select
		//   }
		if len(v.narrowed) > 0 {
			saved := maps.Clone(v.narrowed)
			if stmt.Body != nil {
				for _, s := range stmt.Body.List {
					if cc, ok := s.(*ast.CommClause); ok {
						v.narrowed = maps.Clone(saved)
						if cc.Comm != nil {
							ast.Walk(v, cc.Comm)
						}
						for _, bs := range cc.Body {
							ast.Walk(v, bs)
						}
					} else {
						ast.Walk(v, s)
					}
				}
				v.narrowed = saved
				v.invalidateMutatedIn(stmt.Body)
			}
			return nil
		}

	case *ast.ForStmt:
		// Invalidate variables mutated anywhere in Cond, Post, or Body
		// before walking them, because later loop iterations execute
		// after mutations at the bottom of an earlier iteration:
		//
		//   switch x.(type) {
		//   case string:
		//       for i := 0; i < 2; i++ {
		//           _ = x.(string) // unchecked: x is 42 on iteration 2
		//           x = 42
		//       }
		//   }
		if len(v.narrowed) > 0 {
			if stmt.Init != nil {
				ast.Walk(v, stmt.Init)
			}
			v.invalidateMutatedIn(stmt.Cond)
			v.invalidateMutatedIn(stmt.Post)
			v.invalidateMutatedIn(stmt.Body)
			if stmt.Cond != nil {
				ast.Walk(v, stmt.Cond)
			}
			if stmt.Post != nil {
				ast.Walk(v, stmt.Post)
			}
			if stmt.Body != nil {
				ast.Walk(v, stmt.Body)
			}
			return nil
		}

	case *ast.TypeAssertExpr:
		v.checkAssertExpr(stmt)
		return nil

	case *ast.RangeStmt:
		v.checkRangeStmt(stmt)
		// A range loop assigns to Key and Value on each iteration (e.g.
		// "for x = range vals") and its body can mutate narrowed vars on
		// iteration 1 before iteration 2 runs, so invalidate both before
		// walking the loop body.
		if len(v.narrowed) > 0 {
			ast.Walk(v, stmt.X)
			v.invalidateIdent(stmt.Key)
			v.invalidateIdent(stmt.Value)
			v.invalidateMutatedIn(stmt.Body)
			if stmt.Key != nil {
				ast.Walk(v, stmt.Key)
			}
			if stmt.Value != nil {
				ast.Walk(v, stmt.Value)
			}
			if stmt.Body != nil {
				ast.Walk(v, stmt.Body)
			}
			return nil
		}

	default:
	}
	return v
}

// checkAssignment checks the assignment statement and returns a boolean value
// indicating whether to continue checking the substructure in AssignStmt or not
func (v *visitor) checkAssignment(lhs, rhs []ast.Expr) (followed bool) {
	if len(rhs) == 1 {
		// single value on rhs; check against lhs identifiers
		switch expr := ast.Unparen(rhs[0]).(type) {
		case *ast.CallExpr:
			if !v.checkBlankAssignments {
				return true
			}
			// Find the blank identifiers that would be reported before
			// checking exclusions, since that check is much more expensive
			// and most calls have no error assigned to a blank identifier.
			var blanks []ast.Expr
			isError := v.errorsByArg(expr)
			for i := range lhs {
				// We shortcut calls to recover() because errorsByArg can't
				// check its return types for errors since it returns interface{}.
				if isBlankIdent(lhs[i]) && (v.isRecover(expr) || (i < len(isError) && isError[i])) {
					blanks = append(blanks, lhs[i])
				}
			}
			if len(blanks) > 0 && !v.ignoreCall(expr) {
				for _, blank := range blanks {
					v.addErrorAtPosition(blank.Pos(), expr)
				}
			}
		case *ast.TypeAssertExpr:
			if !v.checkTypeAsserts || expr.Type == nil {
				// Not checking assertions, or this is a type switch.
				return false
			}
			if len(lhs) < 2 {
				// assertion result not read
				if !v.isGuaranteedTypeAssert(expr) {
					v.addErrorAtPosition(expr.Pos(), nil)
				}
			} else if v.checkBlankAssignments && isBlankIdent(lhs[1]) {
				// assertion result ignored
				v.addErrorAtPosition(lhs[1].Pos(), nil)
			}
			return false
		case *ast.UnaryExpr:
			if expr.Op == token.ARROW && v.checkBlankAssignments && len(lhs) >= 1 && isBlankIdent(lhs[0]) && v.isChannelReceiveReturningError(expr) {
				v.addErrorAtPosition(lhs[0].Pos(), nil)
			}
		}
		return true
	}

	// multiple value on rhs; in this case a call can't return
	// multiple values. Assume len(lhs) == len(rhs)
	//
	// Type assertions need no handling here: each one is in its
	// single-value form, so the walk into rhs reports it through
	// checkAssertExpr.
	if !v.checkBlankAssignments {
		return true
	}
	for i := range min(len(lhs), len(rhs)) {
		if !isBlankIdent(lhs[i]) {
			continue
		}
		switch expr := ast.Unparen(rhs[i]).(type) {
		case *ast.CallExpr:
			if v.callReturnsError(expr) && !v.ignoreCall(expr) {
				v.addErrorAtPosition(lhs[i].Pos(), expr)
			}
		case *ast.UnaryExpr:
			if expr.Op == token.ARROW && v.isChannelReceiveReturningError(expr) {
				v.addErrorAtPosition(lhs[i].Pos(), nil)
			}
		}
	}
	return true
}

// isBlankIdent reports whether expr is the blank identifier "_".
func isBlankIdent(expr ast.Expr) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == "_"
}

func (v *visitor) checkAssertExpr(expr *ast.TypeAssertExpr) {
	if !v.checkTypeAsserts {
		return
	}
	if expr.Type == nil {
		// type switch
		return
	}
	if v.isGuaranteedTypeAssert(expr) {
		return
	}
	v.addErrorAtPosition(expr.Pos(), nil)
}

// walkFuncBody saves and clears the visitor's per-function narrowing state
// before walking body, then restores the enclosing function's state.
//
// Resetting narrowed prevents narrowing in an outer function from leaking into
// a nested function literal that may run later:
//
//	switch x.(type) {
//	case string:
//	    go func() { _ = x.(string) }() // x may be reassigned before goroutine runs
//	}
//	x = 42
func (v *visitor) walkFuncBody(fn ast.Node, body *ast.BlockStmt) {
	prevFn := v.currentFn
	prevReady := v.closureInfoReady
	prevClosureMutated := v.closureMutated
	prevNarrowed := v.narrowed

	v.currentFn = fn
	v.closureInfoReady = false
	v.closureMutated = nil
	v.narrowed = nil

	ast.Walk(v, body)

	v.currentFn = prevFn
	v.closureInfoReady = prevReady
	v.closureMutated = prevClosureMutated
	v.narrowed = prevNarrowed
}

// ensureClosureInfo lazily scans nested function literals in currentFn once to
// find local variables that a closure can mutate or take the address of. Such
// variables cannot safely stay narrowed across calls:
//
//	var x any = "hello"
//	reset := func() { x = 42 }
//	switch x.(type) {
//	case string:
//	    reset()        // mutates x even though x does not appear at the call site
//	    _ = x.(string) // unchecked
//	}
func (v *visitor) ensureClosureInfo() {
	if v.closureInfoReady {
		return
	}
	v.closureInfoReady = true
	v.closureMutated = make(map[*types.Var]bool)
	if v.currentFn == nil || v.typesInfo == nil {
		return
	}
	var body *ast.BlockStmt
	switch fn := v.currentFn.(type) {
	case *ast.FuncDecl:
		body = fn.Body
	case *ast.FuncLit:
		body = fn.Body
	}
	if body == nil {
		return
	}
	ast.Inspect(body, func(n ast.Node) bool {
		if fl, ok := n.(*ast.FuncLit); ok {
			v.collectMutatedVars(fl.Body, v.closureMutated)
			return false
		}
		return true
	})
}

// typeSwitchGuardVar returns the local variable switched on by stmt if it is
// safe to track across case clauses, or nil otherwise.
//
// It handles both guard forms:
//   - "switch x.(type)"      (*ast.ExprStmt)
//   - "switch y := x.(type)" (*ast.AssignStmt)
//
// Package-level globals ("var g any"), variables captured from an outer
// function, and variables mutated by a closure in currentFn return nil because
// arbitrary function calls or other goroutines could change their dynamic type
// mid-case.
func (v *visitor) typeSwitchGuardVar(stmt *ast.TypeSwitchStmt) *types.Var {
	if v.currentFn == nil || v.typesInfo == nil || stmt.Assign == nil {
		return nil
	}
	var ta *ast.TypeAssertExpr
	switch assign := stmt.Assign.(type) {
	case *ast.ExprStmt:
		ta, _ = ast.Unparen(assign.X).(*ast.TypeAssertExpr)
	case *ast.AssignStmt:
		if len(assign.Rhs) == 1 {
			ta, _ = ast.Unparen(assign.Rhs[0]).(*ast.TypeAssertExpr)
		}
	}
	if ta == nil || ta.Type != nil {
		return nil
	}
	ident, ok := ast.Unparen(ta.X).(*ast.Ident)
	if !ok {
		return nil
	}
	vObj, ok := v.typesInfo.Uses[ident].(*types.Var)
	if !ok || vObj == nil {
		return nil
	}
	// Restrict to variables declared in the immediately enclosing function
	// (receiver, parameters, results, or locals).
	if vObj.Pos() < v.currentFn.Pos() || vObj.Pos() > v.currentFn.End() {
		return nil
	}
	v.ensureClosureInfo()
	if v.closureMutated[vObj] {
		return nil
	}
	return vObj
}

// walkTypeSwitchStmt walks a type switch while recording per-case type
// constraints on the switched variable:
//
//	switch x.(type) {
//	case string:
//	    _ = x.(string)            // narrowed[x] includes [string] -> safe
//	case *bytes.Buffer, *os.File:
//	    _ = x.(io.Reader)         // narrowed[x] includes [*bytes.Buffer, *os.File] -> safe
//	case nil:
//	    _ = x.(any)               // nil case does not narrow x -> unchecked
//	default:
//	    _ = x.(string)            // default has empty cc.List -> unchecked
//	}
func (v *visitor) walkTypeSwitchStmt(stmt *ast.TypeSwitchStmt) {
	if stmt.Init != nil {
		ast.Walk(v, stmt.Init)
	}
	if stmt.Assign != nil {
		ast.Walk(v, stmt.Assign)
	}
	if stmt.Body == nil {
		return
	}

	afterHeader := maps.Clone(v.narrowed)
	guardVar := v.typeSwitchGuardVar(stmt)

	for _, s := range stmt.Body.List {
		cc, ok := s.(*ast.CaseClause)
		if !ok {
			ast.Walk(v, s)
			continue
		}

		v.narrowed = maps.Clone(afterHeader)
		for _, e := range cc.List {
			ast.Walk(v, e)
		}

		if guardVar != nil && len(cc.List) > 0 {
			caseTypes := make([]types.Type, 0, len(cc.List))
			for _, ce := range cc.List {
				tv, ok := v.typesInfo.Types[ce]
				// Exclude "case nil:", whose dynamic value is nil and panics on x.(T).
				if !ok || tv.IsNil() || tv.Type == nil || tv.Type == types.Typ[types.UntypedNil] {
					caseTypes = nil
					break
				}
				caseTypes = append(caseTypes, tv.Type)
			}
			if len(caseTypes) > 0 {
				if v.narrowed == nil {
					v.narrowed = make(map[*types.Var][][]types.Type)
				}
				v.narrowed[guardVar] = append(slices.Clone(v.narrowed[guardVar]), caseTypes)
			}
		}

		// If the case contains a goto, a backward jump could reach an
		// assertion after mutating a narrowed variable later in the case:
		//
		//   retry:
		//       _ = x.(string) // unchecked: x is 42 after goto retry
		//       x = 42
		//       goto retry
		if len(v.narrowed) > 0 && containsGoto(cc) {
			v.invalidateMutatedIn(cc)
		}

		for _, bs := range cc.Body {
			ast.Walk(v, bs)
		}
	}

	v.narrowed = afterHeader
	v.invalidateMutatedIn(stmt.Body)
}

// isGuaranteedTypeAssert reports whether expr (a single-value assertion like
// "x.(T)") is guaranteed not to panic by an active enclosing type-switch case.
//
// At least one active case on x must guarantee T for every type in that case's
// type list. For example:
//   - "case string:" guarantees "x.(string)"
//   - "case *bytes.Buffer, *strings.Reader:" guarantees "x.(io.Reader)",
//     but NOT "x.(*bytes.Buffer)" because x may hold *strings.Reader.
func (v *visitor) isGuaranteedTypeAssert(expr *ast.TypeAssertExpr) bool {
	if len(v.narrowed) == 0 || v.typesInfo == nil || expr.Type == nil {
		return false
	}
	ident, ok := ast.Unparen(expr.X).(*ast.Ident)
	if !ok {
		return false
	}
	vObj, ok := v.typesInfo.Uses[ident].(*types.Var)
	if !ok {
		return false
	}
	constraints := v.narrowed[vObj]
	if len(constraints) == 0 {
		return false
	}
	assertedType := v.typesInfo.TypeOf(expr.Type)
	if assertedType == nil {
		return false
	}
	for _, caseTypes := range constraints {
		allMatch := len(caseTypes) > 0
		for _, ct := range caseTypes {
			if !typeSwitchCaseGuarantees(ct, assertedType) {
				allMatch = false
				break
			}
		}
		if allMatch {
			return true
		}
	}
	return false
}

// typeSwitchCaseGuarantees reports whether a non-nil value that matched
// caseType in a type switch is guaranteed to succeed in an assertion to
// assertedType without panicking:
//   - Identical types (after unaliasing): "case string:" -> "x.(string)" or
//     "type S = string; x.(S)", whereas "type MySlice []int; case MySlice:"
//     does NOT guarantee "x.([]int)" because defined types are distinct.
//   - Interface satisfaction: "case *bytes.Buffer:" -> "x.(io.Reader)"
//     (types.Implements), whereas "case io.Reader:" does NOT guarantee
//     "x.(*bytes.Buffer)" because the concrete type may differ, and assertions
//     to a type parameter T require exact type identity with caseType.
func typeSwitchCaseGuarantees(caseType, assertedType types.Type) bool {
	if caseType == nil || assertedType == nil || caseType == types.Typ[types.UntypedNil] {
		return false
	}
	caseType = types.Unalias(caseType)
	assertedType = types.Unalias(assertedType)
	if types.Identical(caseType, assertedType) {
		return true
	}
	if _, isTypeParam := assertedType.(*types.TypeParam); isTypeParam {
		return false
	}
	if iface, ok := assertedType.Underlying().(*types.Interface); ok {
		return types.Implements(caseType, iface)
	}
	return false
}

// markIdentMutated records the *types.Var referenced by expr in out when expr
// is an identifier (after stripping parentheses).
func (v *visitor) markIdentMutated(expr ast.Expr, out map[*types.Var]bool) {
	if expr == nil || v.typesInfo == nil {
		return
	}
	if id, ok := ast.Unparen(expr).(*ast.Ident); ok {
		if vObj, ok := v.typesInfo.ObjectOf(id).(*types.Var); ok {
			out[vObj] = true
		}
	}
}

// invalidateIdent removes any active type-switch narrowing for the variable
// referenced by expr (for example after "x = 42" or "&x").
func (v *visitor) invalidateIdent(expr ast.Expr) {
	if len(v.narrowed) == 0 || expr == nil || v.typesInfo == nil {
		return
	}
	if id, ok := ast.Unparen(expr).(*ast.Ident); ok {
		if vObj, ok := v.typesInfo.ObjectOf(id).(*types.Var); ok {
			delete(v.narrowed, vObj)
		}
	}
}

// collectMutatedVars records all variables in node that may be modified by
// assignment ("x = v", "x := v" redeclaration), range iteration
// ("for x = range s"), increment/decrement ("x++"), or address-taking ("&x").
func (v *visitor) collectMutatedVars(node ast.Node, out map[*types.Var]bool) {
	if node == nil || v.typesInfo == nil {
		return
	}
	ast.Inspect(node, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range s.Lhs {
				v.markIdentMutated(lhs, out)
			}
		case *ast.RangeStmt:
			v.markIdentMutated(s.Key, out)
			v.markIdentMutated(s.Value, out)
		case *ast.IncDecStmt:
			v.markIdentMutated(s.X, out)
		case *ast.UnaryExpr:
			if s.Op == token.AND {
				v.markIdentMutated(s.X, out)
			}
		}
		return true
	})
}

// invalidateMutatedIn removes active type-switch narrowing for every variable
// that may be mutated anywhere within node.
func (v *visitor) invalidateMutatedIn(node ast.Node) {
	if len(v.narrowed) == 0 || node == nil {
		return
	}
	mutated := make(map[*types.Var]bool)
	v.collectMutatedVars(node, mutated)
	for vObj := range mutated {
		delete(v.narrowed, vObj)
	}
}

// containsGoto reports whether node contains a "goto" statement outside any
// nested function literal.
func containsGoto(node ast.Node) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if found {
			return false
		}
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		if br, ok := n.(*ast.BranchStmt); ok && br.Tok == token.GOTO {
			found = true
			return false
		}
		return true
	})
	return found
}

// containsFallthrough reports whether any top-level case clause in a switch
// body ends with a "fallthrough" statement:
//
//	switch n {
//	case 1:
//	    x = 42
//	    fallthrough
//	case 2:
//	    _ = x.(string) // x may be 42 via fallthrough from case 1
//	}
func containsFallthrough(body *ast.BlockStmt) bool {
	for _, s := range body.List {
		cc, ok := s.(*ast.CaseClause)
		if !ok {
			continue
		}
		for _, bs := range cc.Body {
			if br, ok := bs.(*ast.BranchStmt); ok && br.Tok == token.FALLTHROUGH {
				return true
			}
		}
	}
	return false
}

// unwrapCall returns the call expression within expr after removing parentheses, or nil if expr is not a call.
func unwrapCall(expr ast.Expr) *ast.CallExpr {
	call, _ := ast.Unparen(expr).(*ast.CallExpr)
	return call
}

// unwrapUnary returns the unary expression within expr after removing parentheses, or nil if expr is not a unary expression.
func unwrapUnary(expr ast.Expr) *ast.UnaryExpr {
	unary, _ := ast.Unparen(expr).(*ast.UnaryExpr)
	return unary
}

// isChannelReceiveReturningError reports whether the unary expression is a receive from a channel that returns an error.
func (v *visitor) isChannelReceiveReturningError(unary *ast.UnaryExpr) bool {
	t := v.typesInfo.TypeOf(ast.Unparen(unary.X))
	if t == nil {
		return false
	}
	ch, ok := t.Underlying().(*types.Chan)
	return ok && isErrorType(ch.Elem())
}

func (v *visitor) checkRangeStmt(stmt *ast.RangeStmt) {
	t := v.typesInfo.TypeOf(stmt.X)
	if t == nil {
		return
	}

	sig, ok := t.Underlying().(*types.Signature)
	if !ok {
		return
	}

	// An iterator function takes a single yield function argument and returns nothing.
	if sig.Params().Len() != 1 || sig.Results().Len() != 0 {
		return
	}

	yieldSig, ok := sig.Params().At(0).Type().Underlying().(*types.Signature)
	if !ok {
		return
	}

	// The yield function returns a single bool.
	if yieldSig.Results().Len() != 1 {
		return
	}
	resBasic, ok := yieldSig.Results().At(0).Type().Underlying().(*types.Basic)
	if !ok || resBasic.Info()&types.IsBoolean == 0 {
		return
	}

	numYieldParams := yieldSig.Params().Len()
	if numYieldParams == 0 || numYieldParams > 2 {
		return
	}

	// Find the errors that would be reported before checking exclusions,
	// since that check is much more expensive.
	var positions []token.Pos
	for i := range numYieldParams {
		if !isErrorType(yieldSig.Params().At(i).Type()) {
			continue
		}

		var expr ast.Expr
		switch i {
		case 0:
			expr = stmt.Key
		case 1:
			expr = stmt.Value
		}

		if expr == nil {
			// The error isn't assigned at all. Any later values aren't
			// either, so report the range statement once.
			positions = append(positions, stmt.Pos())
			break
		}
		if v.checkBlankAssignments && isBlankIdent(expr) {
			positions = append(positions, expr.Pos())
		}
	}
	if len(positions) == 0 {
		return
	}

	call := unwrapCall(stmt.X)
	if call != nil && v.ignoreCall(call) {
		return
	}
	for _, pos := range positions {
		v.addErrorAtPosition(pos, call)
	}
}
