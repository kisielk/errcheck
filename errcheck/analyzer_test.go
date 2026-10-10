package errcheck

import (
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	t.Run("default flags", func(t *testing.T) {
		packageDir := filepath.Join(analysistest.TestData(), "src/a/")
		_ = analysistest.Run(t, packageDir, Analyzer)
	})

	t.Run("check blanks", func(t *testing.T) {
		packageDir := filepath.Join(analysistest.TestData(), "src/blank/")
		_ = Analyzer.Flags.Set("blank", "true")
		_ = analysistest.Run(t, packageDir, Analyzer)
		_ = Analyzer.Flags.Set("blank", "false") // reset it
	})

	t.Run("check asserts", func(t *testing.T) {
		packageDir := filepath.Join(analysistest.TestData(), "src/assert/")
		_ = Analyzer.Flags.Set("assert", "true")
		_ = analysistest.Run(t, packageDir, Analyzer)
		_ = Analyzer.Flags.Set("assert", "false") // reset it
	})
}
