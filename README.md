# errcheck

errcheck finds silently ignored errors in Go code.

![errcheck](https://github.com/kisielk/errcheck/workflows/errcheck/badge.svg)

For all callables that are not in the excluded list, all returned errors must
be either assigned to a variable or explicitly discarded by being assigned to `_`.

Please note that errcheck does not do any further analysis on assigned errors.

For example, it will not complain about this:

```go
err := foo()

// First error is discarded
err = bar()
if err != nil {
    ...
}
```

For such analysis and more, please see [staticcheck](https://staticcheck.dev/).

## Install

    go install github.com/kisielk/errcheck@latest

errcheck requires Go 1.22 or newer.

## Use

For basic usage, just give the package path of interest as the first argument:

    errcheck github.com/kisielk/errcheck/testdata

To check all packages beneath the current directory:

    errcheck ./...

Or check all packages in your `$GOPATH` and `$GOROOT`:

    errcheck all

errcheck also recognizes the following command-line options:

The `-tags` flag takes a space-separated list of build tags, just like `go
build`. If you are using any custom build tags in your code base, you may need
to specify the relevant tags here.

The `-asserts` flag enables checking for ignored type assertion results. It
takes no arguments.

The `-blank` flag enables checking for assignments of errors to the
blank identifier. It takes no arguments.

The `-abspath` flag prints the absolute paths to files with unchecked errors.

The `-mod` flag sets the module download mode to use: `readonly` or `vendor`.

### go/analysis

The package provides `Analyzer` instance that can be used with
[go/analysis](https://pkg.go.dev/golang.org/x/tools/go/analysis) API.

Currently supported flags are `blank`, `assert`, `exclude`, and `excludeonly`.
Just as the API itself, the analyzer is experimental and may change in the
future.

## Excluding functions

Use the `-exclude` flag to specify a path to a file containing a list of functions to
be excluded.

    errcheck -exclude errcheck_excludes.txt path/to/package

The file should contain one function signature per line. The format for function signatures is
`package.FunctionName` while for methods it's `(package.Receiver).MethodName` for value receivers
and `(*package.Receiver).MethodName` for pointer receivers. If the function name is followed by string of form `(TYPE)`, then
the the function call is excluded only if the type of the first argument is `TYPE`. It also accepts a special suffix
`(os.Stdout)` and `(os.Stderr)`, which excludes the function only when the first argument is a literal `os.Stdout` or `os.Stderr`.

An example of an exclude file is:

    io.Copy(*bytes.Buffer)
    io.Copy(os.Stdout)
    os.ReadFile

    // Sometimes we don't care if a HTTP request fails.
    (*net/http.Client).Do

By default, the exclude list is combined with an internal list for functions in
the Go standard library that have an error return type but are documented to never
return an error. To disable the built-in exclude list, pass the `-excludeonly` flag.

Run errcheck in `-verbose` mode to see the resulting list of added excludes.

When using vendored dependencies, specify the full import path. For example:
* Your project's import path is `example.com/yourpkg`
* You've vendored `example.net/fmt2` as `vendor/example.net/fmt2`
* You want to exclude `fmt2.Println` from error checking

In this case, add this line to your exclude file:
```
example.com/yourpkg/vendor/example.net/fmt2.Println
```

Empty lines and lines starting with `//` are ignored.

### The deprecated method

The `-ignore` flag takes a comma-separated list of pairs of the form package:regex.
For each package, the regex describes which functions to ignore within that package.
The package may be omitted to have the regex apply to all packages.

For example, you may wish to ignore common operations like Read and Write:

    errcheck -ignore '[rR]ead|[wW]rite' path/to/package

or you may wish to ignore common functions like the `print` variants in `fmt`:

    errcheck -ignore 'fmt:[FS]?[Pp]rint*' path/to/package

The `-ignorepkg` flag takes a comma-separated list of package import paths
to ignore:

    errcheck -ignorepkg 'fmt,encoding/binary' path/to/package

Note that this is equivalent to:

    errcheck -ignore 'fmt:.*,encoding/binary:.*' path/to/package

If a regex is provided for a package `pkg` via `-ignore`, and `pkg` also appears
in the list of packages passed to `-ignorepkg`, the latter takes precedence;
that is, all functions within `pkg` will be ignored.

Note that by default the `fmt` package is ignored entirely, unless a regex is
specified for it. To disable this, specify a regex that matches nothing:

    errcheck -ignore 'fmt:a^' path/to/package

The `-ignoretests` flag disables checking of `_test.go` files. It takes
no arguments.

The `-ignoregenerated` flag disables checking of generated source code. It takes no arguments.

## Exit Codes

errcheck returns 1 if any problems were found in the checked files.
It returns 2 if there were any other failures.

# Editor Integration

## Emacs

[go-errcheck.el](https://github.com/dominikh/go-errcheck.el)
integrates errcheck with Emacs by providing a `go-errcheck` command
and customizable variables to automatically pass flags to errcheck.

## Vim

[vim-go](https://github.com/fatih/vim-go) can run errcheck via both its `:GoErrCheck`
and `:GoMetaLinter` commands.
