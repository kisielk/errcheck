//go:build go1.26

package main

import (
	"errors"
)

func ignoreErrorsAsType() {
	var dummyErr error
	errors.AsType[MyError](dummyErr)
	_, _ = errors.AsType[MyError](dummyErr)
}
