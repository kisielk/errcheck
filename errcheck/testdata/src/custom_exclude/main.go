package custom_excludes

// Test custom excludes
type ErrorMakerInterface interface {
	MakeNilError() error
}
type ErrorMakerInterfaceWrapper interface {
	ErrorMakerInterface
}

func main() {
	var emiw ErrorMakerInterfaceWrapper
	emiw.MakeNilError() // ok, custom exclude
}
