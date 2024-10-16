package main

import (
	"crypto/sha256"
	"hash/maphash"
)

func ignoreHashReturns() {
	sha256.New().Write([]byte{}) // EXCLUDED
}

func ignoreHashMapReturns() {
	var hasher maphash.Hash
	hasher.Write(nil)      // EXCLUDED
	hasher.WriteByte(0)    // EXCLUDED
	hasher.WriteString("") // EXCLUDED
}
