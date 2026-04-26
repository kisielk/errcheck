//go:build go1.24

package main

import (
	"crypto/sha3"
)

func ignoreSHA3() {
	h := sha3.New256()
	h.Write([]byte("hello world")) // EXCLUDED
}

func ignoreSHA3_SHAKE() {
	h := sha3.NewSHAKE256()
	h.Write([]byte("hello world")) // EXCLUDED
	buf := make([]byte, 32)
	h.Read(buf) // EXCLUDED
}
