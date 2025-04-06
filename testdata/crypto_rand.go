package main

import (
	"crypto/rand"
)

func ignoreRandReaderReturns() {
	buf := make([]byte, 128)
	rand.Read(buf) // EXCLUDED
}
