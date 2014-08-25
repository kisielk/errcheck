package errcheck

import (
	"fmt"
	"go/build"
	"path/filepath"
)

// findPackage finds a package.
// path is first tried as an import path and if the package is not found, as a filesystem path.
func findPackage(path string) (*build.Package, error) {
	var (
		err1, err2 error
		pkg        *build.Package
	)

	ctx := build.Default
	ctx.CgoEnabled = false

	// First try to treat path as import path...
	pkg, err1 = ctx.Import(path, ".", 0)
	if err1 != nil {
		if _, ok := err1.(*build.NoGoError); ok {
			return nil, err1
		}
		// ... then attempt as file path
		pkg, err2 = ctx.ImportDir(path, 0)
	}

	if err2 != nil {
		// Print both errors so the user can see in what ways the
		// package lookup failed.
		return nil, fmt.Errorf("could not import %s: %s\n%s", path, err1, err2)
	}

	return pkg, nil
}

// getFiles returns all the Go files found in a package
func getFiles(pkg *build.Package) []string {
	files := make([]string, len(pkg.GoFiles))
	for i, fileName := range pkg.GoFiles {
		files[i] = filepath.Join(pkg.Dir, fileName)
	}
	return files
}
