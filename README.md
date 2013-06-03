errcheck
========

errcheck is a program for checking for unchecked errors in go programs.

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

The `-ignore` flag takes a regular expression of function names to ignore.
For example, you may wish to ignore common operations like Read and Write:

    errcheck -ignore '[rR]ead|[wW]rite' path/to/package

The `-ignorepkg` flag takes a comma-separated list of package import paths
to ignore. By default the `fmt` package is ignored, so you should include
it in your list if you want the default behavior:

    errcheck -ignorepkg 'fmt,encoding/binary' path/to/package

The `-blank` flag enables checking for assignments of errors to the
blank identifier. It takes no arguments.

An example of using errcheck to check the go standard library packages:

    go list std | grep -v cmd | xargs -n 1 errcheck -ignore 'Close|[wW]rite.*|Flush|Seek|[rR]ead.*'> stdlibcheck

Exit Codes
----------

errcheck returns 1 if any problems were found in the checked files.
It returns 2 on any if there were any other failures.

Editor Integration
==================

Emacs
-----

[go-errcheck.el](https://github.com/dominikh/go-errcheck.el)
integrates errcheck with Emacs by providing a `go-errcheck` command
and customizable variables to automatically pass flags to errcheck.
