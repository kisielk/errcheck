package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
)

func testfprintf() {
	f, err := os.Create("/tmp/blah")
	if err != nil {
		panic(err)
	}
	buf := bytes.Buffer{}
	s := strings.Builder{}
	fmt.Fprintln(f, "blah") // UNCHECKED
	fmt.Fprintln(os.Stderr, "blah")
	fmt.Fprintln(&buf, "blah")
	fmt.Fprintln(&s, "blah")
	fmt.Println("blah")
}
