package main

import "hash/maphash"


func ignoreHashMapReturns() {
	var hasher maphash.Hash
	hasher.Write(nil)      // EXCLUDED
	hasher.WriteByte(0)    // EXCLUDED
	hasher.WriteString("") // EXCLUDED
}
