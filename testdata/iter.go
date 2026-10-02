package main

import (
	"iter"
)

func fallibleIter() iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		yield("item", nil)
	}
}

func singleErrIter() iter.Seq[error] {
	return func(yield func(error) bool) {
		yield(nil)
	}
}

func customErrIter() iter.Seq2[int, *MyPointerError] {
	return func(yield func(int, *MyPointerError) bool) {
		yield(0, nil)
	}
}

func nonErrIter() iter.Seq2[string, int] {
	return func(yield func(string, int) bool) {
		yield("a", 1)
	}
}

func testIterators() {
	for s := range fallibleIter() { // UNCHECKED
		_ = s
	}

	for range fallibleIter() { // UNCHECKED
	}

	for _ := range fallibleIter() { // UNCHECKED
	}

	for s, _ := range fallibleIter() { // BLANK
		_ = s
	}

	for _, _ := range fallibleIter() { // BLANK
	}

	for s, err := range fallibleIter() {
		_, _ = s, err
	}

	for _, err := range fallibleIter() {
		_ = err
	}

	for range singleErrIter() { // UNCHECKED
	}

	for _ := range singleErrIter() { // BLANK
	}

	for err := range singleErrIter() {
		_ = err
	}

	for i := range customErrIter() { // UNCHECKED
		_ = i
	}

	for i, _ := range customErrIter() { // BLANK
		_ = i
	}

	for i, err := range customErrIter() {
		_, _ = i, err
	}

	for s := range nonErrIter() {
		_ = s
	}

	for s, _ := range nonErrIter() {
		_ = s
	}

	for range nonErrIter() {
	}
}
