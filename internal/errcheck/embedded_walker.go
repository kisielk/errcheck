package errcheck

import (
	// "fmt"
	"go/types"
)

// walkThroughEmbeddedInterfaces returns a slice of types that we need to walk through
// in order to reach the actual interface definition of the function on the other end of this selection (x.f)
//
// False will be returned it:
//   - the left side of the selection is not a function
//   - the right side of the selection is an invalid type
//   - we don't end at an interface-defined function
//
func walkThroughEmbeddedInterfaces(sel *types.Selection) ([]types.Type, bool) {
	fn, ok := sel.Obj().(*types.Func)
	if !ok {
		return nil, false
	}

	currentT := sel.Recv()
	if currentT == types.Typ[types.Invalid] {
		return nil, false
	}

	// The first type is the immediate receiver itself
	result := []types.Type{currentT}

	// First, we can walk through any Struct fields provided
	// by the selection Index() method.
	indexes := sel.Index()
	for _, fieldIndex := range indexes[:len(indexes)-1] {
		currentT = maybeUnname(maybeDereference(currentT))

		// Because we have an entry in Index for this type,
		// we know it has to be a Struct.
		s, ok := currentT.(*types.Struct)
		if !ok {
			panic("expected Struct!")
		}

		nextT := s.Field(fieldIndex).Type()
		result = append(result, nextT)
		currentT = nextT
	}

	// Now currentT is either a Struct implementing the
	// actual function or an interface. If it's an interface,
	// we need to continue digging until we find the interface
	// that actually explicitly defines the function!
	//
	// If it's a Struct, we return false; we're only interested in interface-defined
	// functions here.
	_, ok = maybeUnname(currentT).(*types.Interface)
	if !ok {
		return nil, false
	}

	for {
		interfaceT := maybeUnname(currentT).(*types.Interface)
		if explicitlyDefinesMethod(interfaceT, fn) {
			// then we're done
			break
		}

		// otherwise, search through the embedded interfaces to find
		// the one that defines this method.
		for i := 0; i < interfaceT.NumEmbeddeds(); i++ {
			nextNamedInterface := interfaceT.Embedded(i)
			if definesMethod(maybeUnname(nextNamedInterface).(*types.Interface), fn) {
				result = append(result, nextNamedInterface)
				currentT = nextNamedInterface
				break
			}
		}
	}

	return result, true
}

func explicitlyDefinesMethod(interfaceT *types.Interface, fn *types.Func) bool {
	for i := 0; i < interfaceT.NumExplicitMethods(); i++ {
		if interfaceT.ExplicitMethod(i).Id() == fn.Id() {
			return true
		}
	}
	return false
}

func definesMethod(interfaceT *types.Interface, fn *types.Func) bool {
	for i := 0; i < interfaceT.NumMethods(); i++ {
		if interfaceT.Method(i).Id() == fn.Id() {
			return true
		}
	}
	return false
}

func maybeDereference(t types.Type) types.Type {
	p, ok := t.(*types.Pointer)
	if ok {
		return p.Elem()
	}
	return t
}

func maybeUnname(t types.Type) types.Type {
	n, ok := t.(*types.Named)
	if ok {
		return n.Underlying()
	}
	return t
}
