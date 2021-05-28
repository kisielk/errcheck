package errcheck

import (
	"reflect"
	"testing"
)

func TestReadExcludes(t *testing.T) {
	expectedExcludes := []string{
		"hello()",
		"world()",
	}
	t.Logf("expectedExcludes: %#v", expectedExcludes)
	excludes, err := ReadExcludes("testdata/excludes.txt")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("excludes: %#v", excludes)
	if !reflect.DeepEqual(expectedExcludes, excludes) {
		t.Fatal("excludes did not match expectedExcludes")
	}
}

func TestReadEmptyExcludes(t *testing.T) {
	excludes, err := ReadExcludes("testdata/empty_excludes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(excludes) != 0 {
		t.Fatalf("expected empty excludes, got %#v", excludes)
	}
}

func TestReadExcludesMissingFile(t *testing.T) {
	_, err := ReadExcludes("testdata/missing_file")
	if err == nil {
		t.Fatal("expected non-nil err, got nil")
	}
}
