package main

import (
	"crypto/sha256"
	"hash"
	"hash/maphash"
)

// HashAlias is an alias of hash.Hash, so calls through it must match the
// "(hash.Hash).Write" exclusion just like calls through hash.Hash.
type HashAlias = hash.Hash

// hashAliasHolder embeds the alias, so Write is reached through a field.
type hashAliasHolder struct {
	HashAlias
}

func ignoreHashReturns() {
	sha256.New().Write([]byte{}) // EXCLUDED
}

func ignoreHashAliasReturns() {
	var h HashAlias = sha256.New()
	h.Write(nil) // EXCLUDED

	holder := hashAliasHolder{sha256.New()}
	holder.Write(nil) // EXCLUDED
}

func ignoreHashMapReturns() {
	var hasher maphash.Hash
	hasher.Write(nil)      // EXCLUDED
	hasher.WriteByte(0)    // EXCLUDED
	hasher.WriteString("") // EXCLUDED
}
