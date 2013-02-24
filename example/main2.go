// This file exists so that we can check that multi-file packages work
package main

import "fmt"

type t struct{}

func (x t) a() error {
	fmt.Println("this method returns an error")
	return nil
}

type u struct {
	t t
}
