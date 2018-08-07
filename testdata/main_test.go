package main

import (
	"bytes"
	"crypto/sha256"
	"io/ioutil"
	"math/rand"
	mrand "math/rand"

	"testing"
)

func TestFunc(tt *testing.T) {
	// Single error return
	_ = a() // BLANK
	a()     // UNCHECKED

	// Return another value and an error
	_, _ = b() // BLANK
	b()        // UNCHECKED

	// Return a custom error type
	_ = customError() // BLANK
	customError()     // UNCHECKED

	// Return a custom concrete error type
	_ = customConcreteError()         // BLANK
	customConcreteError()             // UNCHECKED
	_, _ = customConcreteErrorTuple() // BLANK
	customConcreteErrorTuple()        // UNCHECKED

	// Return a custom pointer error type
	_ = customPointerError()         // BLANK
	customPointerError()             // UNCHECKED
	_, _ = customPointerErrorTuple() // BLANK
	customPointerErrorTuple()        // UNCHECKED

	// Method with a single error return
	x := t{}
	_ = x.a() // BLANK
	x.a()     // UNCHECKED

	// Method call on a struct member
	y := u{x}
	_ = y.t.a() // BLANK
	y.t.a()     // UNCHECKED

	m1 := map[string]func() error{"a": a}
	_ = m1["a"]() // BLANK
	m1["a"]()     // UNCHECKED

	// Additional cases for assigning errors to blank identifier
	z, _ := b()    // BLANK
	_, w := a(), 5 // BLANK

	// Assign non error to blank identifier
	_ = c()

	_ = z + w // Avoid complaints about unused variables

	// Type assertions
	var i interface{}
	s1 := i.(string)    // ASSERT
	s1 = i.(string)     // ASSERT
	s2, _ := i.(string) // ASSERT
	s2, _ = i.(string)  // ASSERT
	s3, ok := i.(string)
	s3, ok = i.(string)
	switch s4 := i.(type) {
	case string:
		_ = s4
	}
	_, _, _, _ = s1, s2, s3, ok

	// Goroutine
	go a()    // UNCHECKED
	defer a() // UNCHECKED

	b1 := bytes.Buffer{}
	b2 := &bytes.Buffer{}
	b1.Write(nil)
	b2.Write(nil)
	rand.Read(nil)
	mrand.Read(nil)
	sha256.New().Write([]byte{})

	ioutil.ReadFile("main.go") // UNCHECKED

	var emiw ErrorMakerInterfaceWrapper
	emiw.MakeNilError()
}
