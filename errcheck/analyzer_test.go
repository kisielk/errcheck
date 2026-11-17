package errcheck

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// setFlag sets an Analyzer flag for the rest of the test, then resets it.
func setFlag(t *testing.T, name, value string) {
	t.Helper()
	old := Analyzer.Flags.Lookup(name).Value.String()
	if err := Analyzer.Flags.Set(name, value); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Analyzer.Flags.Set(name, old) })
}

func TestAnalyzer(t *testing.T) {
	t.Run("default flags", func(t *testing.T) {
		packageDir := filepath.Join(analysistest.TestData(), "src/a/")
		_ = analysistest.Run(t, packageDir, Analyzer)
	})

	t.Run("check blanks", func(t *testing.T) {
		packageDir := filepath.Join(analysistest.TestData(), "src/blank/")
		setFlag(t, "blank", "true")
		_ = analysistest.Run(t, packageDir, Analyzer)
	})

	t.Run("check asserts", func(t *testing.T) {
		packageDir := filepath.Join(analysistest.TestData(), "src/assert/")
		setFlag(t, "asserts", "true")
		_ = analysistest.Run(t, packageDir, Analyzer)
	})

	t.Run("check asserts with deprecated flag", func(t *testing.T) {
		packageDir := filepath.Join(analysistest.TestData(), "src/assert/")
		setFlag(t, "assert", "true")
		_ = analysistest.Run(t, packageDir, Analyzer)
	})

	t.Run("custom exclude", func(t *testing.T) {
		packageDir := filepath.Join(analysistest.TestData(), "src/custom_exclude")
		// analysistest loads a package outside GOPATH, so its path is the
		// directory prefixed with "_".
		symbol := "(_" + filepath.ToSlash(packageDir) + ".ErrorMakerInterface).MakeNilError"
		excludeFile := filepath.Join(t.TempDir(), "excludes.txt")
		if err := os.WriteFile(excludeFile, []byte(symbol+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
		setFlag(t, "exclude", excludeFile)
		_ = analysistest.Run(t, packageDir, Analyzer)
	})
}

// TestExcludedSymbols checks that the exclude file is read once and cached
// while the flags stay the same, and that read errors are wrapped.
func TestExcludedSymbols(t *testing.T) {
	excludeFile := filepath.Join(t.TempDir(), "excludes.txt")
	if err := os.WriteFile(excludeFile, []byte("example.com/pkg.F\n"), 0644); err != nil {
		t.Fatal(err)
	}
	setFlag(t, "exclude", excludeFile)

	first, err := excludedSymbols()
	if err != nil {
		t.Fatal(err)
	}
	if !first["example.com/pkg.F"] {
		t.Errorf("symbol from exclude file is missing")
	}

	// The file is only read once, so removing it doesn't matter now.
	if err := os.Remove(excludeFile); err != nil {
		t.Fatal(err)
	}
	if _, err := excludedSymbols(); err != nil {
		t.Errorf("exclude file was read again: %v", err)
	}

	// Changing a flag rebuilds the set, which now fails to read the file.
	setFlag(t, "excludeonly", "true")
	_, err = excludedSymbols()
	if err == nil {
		t.Fatal("expected an error reading the removed exclude file")
	}
	if !strings.HasPrefix(err.Error(), "could not read exclude file: ") || !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("unexpected error: %v", err)
	}
}
