errcheck
========

errcheck is a program for checking for unchecked errors in go programs.

[![Build Status](https://drone.io/github.com/kisielk/errcheck/status.png)](https://drone.io/github.com/kisielk/errcheck/latest)

Install
-------

    go get github.com/kisielk/errcheck

errcheck requires Go 1.1 and depends on the go/types package from the go.tools repository.

Use
---

For basic usage, just give the package path of interest as the first
argument:

    errcheck github.com/kisielk/errcheck/example

There are currently three flags: `-ignore`, `-ignorepkg` and `-blank`

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

The `-blank` flag enables checking for assignments of errors to the
blank identifier. It takes no arguments.

An example of using errcheck to check the go standard library packages:

    errcheck -ignore 'Close|[wW]rite.*|Flush|Seek|[rR]ead.*' std > stdlibcheck

Or check all packages in your $GOPATH and $GOROOT:

    errcheck all > allcheck

To check all packages beneath the current directory:

    errcheck ./...

Exit Codes
----------

errcheck returns 1 if any problems were found in the checked files.
It returns 2 if there were any other failures.

Editor Integration
==================

Emacs
-----

[go-errcheck.el](https://github.com/dominikh/go-errcheck.el)
integrates errcheck with Emacs by providing a `go-errcheck` command
and customizable variables to automatically pass flags to errcheck.
