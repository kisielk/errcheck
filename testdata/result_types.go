package main

// Results whose type is not a named or pointer type are still checked when
// they implement error.

func typeParamErr[E error](f func() E) {
	f()     // UNCHECKED
	_ = f() // BLANK
}

func typeParamErrTuple[E error](f func() (int, E)) {
	f()        // UNCHECKED
	_, _ = f() // BLANK
}

func typeParamAny[A any](f func() A) {
	f()
	_ = f()
}

func anonErrIface() interface{ Error() string } { return nil }

func anonEmbedErrIface() interface {
	error
	Code() int
} {
	return nil
}

func anonEmptyIface() interface{} { return nil }

func checkResultTypes() {
	anonErrIface()          // UNCHECKED
	_ = anonErrIface()      // BLANK
	anonEmbedErrIface()     // UNCHECKED
	_ = anonEmbedErrIface() // BLANK
	anonEmptyIface()
	_ = anonEmptyIface()
}
