package errcheck

import (
	"golang.org/x/tools/go/analysis/analysistest"
	"path/filepath"
	"testing"
)

func TestAnalyzer(t *testing.T) {
	packageDir := filepath.Join(analysistest.TestData(), "src/a/")
	_ = analysistest.Run(t, packageDir, Analyzer)
}
