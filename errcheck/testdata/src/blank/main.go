package blank

import (
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

	for s := range fallibleSeq() { // want "unchecked error"
		_ = s
	}

	for s, _ := range fallibleSeq() { // want "unchecked error"
		_ = s
	}

	for s, err := range fallibleSeq() {
		_, _ = s, err
	}

	errChan := make(chan error)
	intChan := make(chan int)

	<-errChan          // want "unchecked error"
	_ = <-errChan      // want "unchecked error"
	_, ok := <-errChan // want "unchecked error"
	_ = ok
	var _ = <-errChan  // want "unchecked error"

	errVal := <-errChan
	_ = errVal
	errVal2, _ := <-errChan
	_ = errVal2

	<-intChan
	_ = <-intChan
	_, ok2 := <-intChan
	_ = ok2
}

// https://github.com/kisielk/errcheck/issues/230: a type conversion to a type that happens to implement error
// (e.g. as a compile-time interface satisfaction check) must not be treated
// as an unchecked error return - it isn't a function call at all.
type convError struct{}

func (convError) Error() string { return "boom" }

var _ error = (*convError)(nil) // ignored, this is a type conversion, not a call
