// ensure that the package keyword is not equal to file beginning
// to test correct position calculations.
package a

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math/rand"
	mrand "math/rand"
	"os"
)

func a() error {
	fmt.Println("this function returns an error") // ok, excluded
	return nil
}

func b() (int, error) {
	fmt.Println("this function returns an int and an error") // ok, excluded
	return 0, nil
}

func c() int {
	fmt.Println("this function returns an int") // ok, excluded
	return 7
}

func rec() {
	defer func() {
		recover()     // want "unchecked error"
		_ = recover() // ok, assigned to blank
	}()
	defer recover() // want "unchecked error"
}

type MyError string

func (e MyError) Error() string {
	return string(e)
}

func customError() error {
	return MyError("an error occurred")
}

func customConcreteError() MyError {
	return MyError("an error occurred")
}

func customConcreteErrorTuple() (int, MyError) {
	return 0, MyError("an error occurred")
}

type MyPointerError string

func (e *MyPointerError) Error() string {
	return string(*e)
}

func customPointerError() *MyPointerError {
	e := MyPointerError("an error occurred")
	return &e
}

func customPointerErrorTuple() (int, *MyPointerError) {
	e := MyPointerError("an error occurred")
	return 0, &e
}

type ErrorMakerInterface interface {
	MakeNilError() error
}
type ErrorMakerInterfaceWrapper interface {
	ErrorMakerInterface
}

func main() {
	// Single error return
	_ = a() // ok, assigned to blank
	a()     // want "unchecked error"

	// Return another value and an error
	_, _ = b() // ok, assigned to blank
	b()        // want "unchecked error"

	// Return a custom error type
	_ = customError() // ok, assigned to blank
	customError()     // want "unchecked error"

	// Return a custom concrete error type
	_ = customConcreteError()         // ok, assigned to blank
	customConcreteError()             // want "unchecked error"
	_, _ = customConcreteErrorTuple() // ok, assigned to blank
	customConcreteErrorTuple()        // want "unchecked error"

	// Return a custom pointer error type
	_ = customPointerError()         // ok, assigned to blank
	customPointerError()             // want "unchecked error"
	_, _ = customPointerErrorTuple() // ok, assigned to blank
	customPointerErrorTuple()        // want "unchecked error"

	// Method with a single error return
	x := t{}
	_ = x.a() // ok, assigned to blank
	x.a()     // want "unchecked error"

	// Method call on a struct member
	y := u{x}
	_ = y.t.a() // ok, assigned to blank
	y.t.a()     // want "unchecked error"

	m1 := map[string]func() error{"a": a}
	_ = m1["a"]() // ok, assigned to blank
	m1["a"]()     // want "unchecked error"

	// Additional cases for assigning errors to blank identifier
	z, _ := b()    // ok, assigned to blank
	_, w := a(), 5 // ok, assigned to blank

	// Assign non error to blank identifier
	_ = c()

	_ = z + w // Avoid complaints about unused variables

	// Type assertions
	var i interface{}
	s1 := i.(string)    // ok, would fail with -assert
	s1 = i.(string)     // ok, would fail with -assert
	s2, _ := i.(string) // ok, would fail with -blank
	s2, _ = i.(string)  // ok, would fail with -blank
	s3, ok := i.(string)
	s3, ok = i.(string)
	switch s4 := i.(type) {
	case string:
		_ = s4
	}
	_, _, _, _ = s1, s2, s3, ok

	// Goroutine
	go a()    // want "unchecked error"
	defer a() // want "unchecked error"

	// IO errors excluded by default
	b1 := bytes.Buffer{}
	b2 := &bytes.Buffer{}
	b1.Write(nil)
	b2.Write(nil)
	rand.Read(nil)
	mrand.Read(nil)
	sha256.New().Write([]byte{})

	os.ReadFile("main.go") // want "unchecked error"

	var emiw ErrorMakerInterfaceWrapper
	emiw.MakeNilError() // want "unchecked error"
}
