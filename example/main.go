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
	_ = a()
	a()
	_, _ = b()
	b()
}
