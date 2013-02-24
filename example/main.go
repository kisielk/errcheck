package main

import "fmt"

func a() error {
	fmt.Println("this function returns an error")
	return nil
}

func b() (int, error) {
	fmt.Println("this function returns an int and an error")
	return 0, nil
}

func main() {
	// Single error return
	_ = a()
	a()

	// Return another value and an error
	_, _ = b()
	b()

	// Method with a single error return
	x := t{}
	_ = x.a()
	x.a()

	// Method call on a struct member
	y := u{x}
	_ = y.t.a()
	y.t.a()
}
