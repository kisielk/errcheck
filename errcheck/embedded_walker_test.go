package errcheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

// commonSrc is a package of structs and interfaces that embed each other,
// shared by the tests in this file. It is a complete file on its own, so
// tests can append more declarations to it.
const commonSrc = `
package p

type Inner struct {}
func (Inner) Method()

type Outer struct {Inner}
type OuterP struct {*Inner}

type InnerInterface interface {
	Method()
}

type OuterInterface interface {InnerInterface}
type MiddleInterfaceStruct struct {OuterInterface}
type OuterInterfaceStruct struct {MiddleInterfaceStruct}

type EmptyInterface interface {}

type SameSignatureInterface interface {
	Method()
}

type InnerInterfaceAlias = InnerInterface
type OuterInterfaceAlias = OuterInterface
type AliasFieldStruct struct {OuterInterfaceAlias}
type UnnamedInterfaceAlias = interface {Method()}
`

// testCase is a TestWalkThroughEmbeddedInterfaces case. selector is the
// expression to type-check, expectedOk is the expected second result of
// walkThroughEmbeddedInterfaces, and expected lists the types it should
// return, in order.
type testCase struct {
	selector   string
	expectedOk bool
	expected   []string
}

// TestWalkThroughEmbeddedInterfaces checks the interfaces that
// walkThroughEmbeddedInterfaces walks through for each selector expression
// appended to commonSrc.
func TestWalkThroughEmbeddedInterfaces(t *testing.T) {
	cases := []testCase{
		{"Inner{}.Method", false, nil},
		{"(&Inner{}).Method", false, nil},
		{"Outer{}.Method", false, nil},
		{"InnerInterface.Method", true, []string{"test.InnerInterface"}},
		{"OuterInterface.Method", true, []string{"test.OuterInterface", "test.InnerInterface"}},
		{"OuterInterfaceStruct.Method", true, []string{"test.OuterInterface", "test.InnerInterface"}},
		// Aliases are looked through, and the aliased types are returned.
		{"InnerInterfaceAlias.Method", true, []string{"test.InnerInterface"}},
		{"OuterInterfaceAlias.Method", true, []string{"test.OuterInterface", "test.InnerInterface"}},
		{"AliasFieldStruct.Method", true, []string{"test.OuterInterface", "test.InnerInterface"}},
		{"UnnamedInterfaceAlias.Method", true, []string{"interface{Method()}"}},
	}

	for _, c := range cases {
		info := types.Info{
			Selections: make(map[*ast.SelectorExpr]*types.Selection),
		}
		_, f := typeCheck(t, commonSrc+"var c = "+c.selector, &info)
		ast.Inspect(f, func(n ast.Node) bool {
			s, ok := n.(*ast.SelectorExpr)
			if ok {
				selection, ok := info.Selections[s]
				if !ok {
					t.Fatalf("no Selection!")
				}
				ts, ok := walkThroughEmbeddedInterfaces(selection)
				if ok != c.expectedOk {
					t.Errorf("expected ok %v got %v", c.expectedOk, ok)
					return false
				}
				if !ok {
					return false
				}

				if len(ts) != len(c.expected) {
					t.Fatalf("expected %d types, got %d", len(c.expected), len(ts))
				}

				for i, e := range c.expected {
					if e != ts[i].String() {
						t.Errorf("mismatch at index %d: expected %s got %s", i, e, ts[i])
					}
				}
			}

			return true
		})

	}

}

// definesMethodFixture holds the interfaces and methods declared in commonSrc.
type definesMethodFixture struct {
	inner         *types.Interface
	outer         *types.Interface
	empty         *types.Interface
	sameSignature *types.Interface

	innerMethod         *types.Func
	sameSignatureMethod *types.Func
}

// definesMethodCase is a case for runDefinesMethodCases: it checks whether
// interfaceT defines fn, expecting the result expected.
type definesMethodCase struct {
	name       string
	interfaceT *types.Interface
	fn         *types.Func
	expected   bool
}

// typeCheck parses and type-checks source as package "test".
func typeCheck(t *testing.T, source string, info *types.Info) (*types.Package, *ast.File) {
	t.Helper()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test", source, 0)
	if err != nil {
		t.Fatal(err)
	}

	conf := types.Config{}
	pkg, err := conf.Check("test", fset, []*ast.File{f}, info)
	if err != nil {
		t.Fatal(err)
	}

	return pkg, f
}

// interfaceByName returns the interface underlying the named type name in pkg.
// It fails the test if there is no such type or it is not an interface.
func interfaceByName(t *testing.T, pkg *types.Package, name string) *types.Interface {
	t.Helper()

	obj := pkg.Scope().Lookup(name)
	if obj == nil {
		t.Fatalf("type %s not found", name)
	}

	named, ok := obj.Type().(*types.Named)
	if !ok {
		t.Fatalf("type %s is %T, want *types.Named", name, obj.Type())
	}

	interfaceT, ok := named.Underlying().(*types.Interface)
	if !ok {
		t.Fatalf("underlying type of %s is %T, want *types.Interface", name, named.Underlying())
	}

	return interfaceT
}

// methodByName returns the method called name from interfaceT's method set,
// including methods it gets from embedded interfaces. It fails the test if
// there is no such method.
func methodByName(t *testing.T, interfaceT *types.Interface, name string) *types.Func {
	t.Helper()

	for method := range interfaceT.Methods() {
		if method.Name() == name {
			return method
		}
	}

	t.Fatalf("method %s not found", name)
	return nil
}

// loadDefinesMethodFixture type-checks commonSrc and returns the interfaces
// and methods used by the definesMethod and explicitlyDefinesMethod tests.
func loadDefinesMethodFixture(t *testing.T) definesMethodFixture {
	t.Helper()

	pkg, _ := typeCheck(t, commonSrc, nil)

	fx := definesMethodFixture{
		inner:         interfaceByName(t, pkg, "InnerInterface"),
		outer:         interfaceByName(t, pkg, "OuterInterface"),
		empty:         interfaceByName(t, pkg, "EmptyInterface"),
		sameSignature: interfaceByName(t, pkg, "SameSignatureInterface"),
	}
	fx.innerMethod = methodByName(t, fx.inner, "Method")
	fx.sameSignatureMethod = methodByName(t, fx.sameSignature, "Method")

	return fx
}

// runDefinesMethodCases runs each case as a subtest, checking that check
// returns the expected result for the case's interface and method.
func runDefinesMethodCases(t *testing.T, check func(*types.Interface, *types.Func) bool, cases []definesMethodCase) {
	t.Helper()

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := check(c.interfaceT, c.fn)
			if got != c.expected {
				t.Errorf("expected %v got %v", c.expected, got)
			}
		})
	}
}

// TestDefinesMethod checks that definesMethod finds methods an interface
// declares itself and methods it gets from embedded interfaces.
func TestDefinesMethod(t *testing.T) {
	fx := loadDefinesMethodFixture(t)

	runDefinesMethodCases(t, definesMethod, []definesMethodCase{
		{"direct", fx.inner, fx.innerMethod, true},
		{"embedded", fx.outer, fx.innerMethod, true},
		{"empty", fx.empty, fx.innerMethod, false},
		{"same signature", fx.inner, fx.sameSignatureMethod, false},
		{"nil func", fx.inner, nil, false},
	})
}

// TestExplicitlyDefinesMethod checks that explicitlyDefinesMethod finds only
// methods an interface declares itself, not ones from embedded interfaces.
func TestExplicitlyDefinesMethod(t *testing.T) {
	fx := loadDefinesMethodFixture(t)

	runDefinesMethodCases(t, explicitlyDefinesMethod, []definesMethodCase{
		{"direct", fx.inner, fx.innerMethod, true},
		{"embedded", fx.outer, fx.innerMethod, false},
		{"empty", fx.empty, fx.innerMethod, false},
		{"same signature", fx.inner, fx.sameSignatureMethod, false},
		{"nil func", fx.inner, nil, false},
	})
}
