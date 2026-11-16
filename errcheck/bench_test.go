package errcheck

import (
	"testing"

	"golang.org/x/tools/go/packages"
)

// benchPackages are large standard library and toolchain packages. ssa is
// mostly generated rewrite rules, so it is dominated by call expressions.
var benchPackages = []string{
	"cmd/compile/internal/ssa",
	"go/types",
	"net/http",
	"runtime",
}

// BenchmarkCheckPackage measures CheckPackage alone: packages are loaded
// once, before the timed loop, so loading and type-checking aren't counted.
func BenchmarkCheckPackage(b *testing.B) {
	configs := []struct {
		name  string
		blank bool // check blank assignments and type assertions
	}{
		{"default", false},
		{"all", true},
	}

	for _, path := range benchPackages {
		var loader Checker
		pkgs, err := loader.LoadPackages(path)
		if err != nil {
			b.Fatal(err)
		}
		packages.Visit(pkgs, nil, func(pkg *packages.Package) {
			if len(pkg.Errors) > 0 {
				b.Fatalf("errors loading %s: %v", pkg.ID, pkg.Errors)
			}
		})

		for _, cfg := range configs {
			var checker Checker
			checker.Exclusions.Symbols = DefaultExcludedSymbols
			checker.Exclusions.BlankAssignments = !cfg.blank
			checker.Exclusions.TypeAssertions = !cfg.blank

			b.Run(path+"/"+cfg.name, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					for _, pkg := range pkgs {
						checker.CheckPackage(pkg)
					}
				}
			})
		}
	}
}
