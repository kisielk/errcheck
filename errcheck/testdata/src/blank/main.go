package blank

import (
	"errors"
	"fmt"
	"iter"
)

func a() error {
	return nil
}

func b() (string, error) {
	return "", nil
}

func c() string {
	return ""
}

func fallibleSeq() iter.Seq2[string, error] {
	return nil
}

func main() {
	_ = a() // want "unchecked error"
	a()     // want "unchecked error"
	b()     // want "unchecked error"
	c()     // ignored, doesn't return an error

	{
		r, err := b() // fine, we're checking the error
		fmt.Printf("r = %v, err = %v\n", r, err)
	}

	{
		r, _ := b() // want "unchecked error"
		fmt.Printf("r = %v\n", r)
	}

	{
		var r, _ = b() // want "unchecked error"
		fmt.Printf("r = %v\n", r)
	}

	{
		if _, ok := errors.AsType[*convError](convError{}); ok {
			_ = ok
		}
	}

	for s := range fallibleSeq() { // want "unchecked error"
		_ = s
	}

	for s, _ := range fallibleSeq() { // want "unchecked error"
		_ = s
	}

	for s, err := range fallibleSeq() {
		_, _ = s, err
	}
}

// https://github.com/kisielk/errcheck/issues/230: a type conversion to a type that happens to implement error
// (e.g. as a compile-time interface satisfaction check) must not be treated
// as an unchecked error return - it isn't a function call at all.
type convError struct{}

func (convError) Error() string { return "boom" }

var _ error = (*convError)(nil) // ignored, this is a type conversion, not a call
