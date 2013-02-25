errcheck
=========

errcheck is a program for checking for unchecked errors in go programs.

Install
-------

    go get github.com/kisielk/errcheck

Note that errcheck depends on the go/types package which is currently only
available in go tip.

Use
---

For basic usage, just give the package path of interest as the first
argument:

    errcheck github.com/kisielk/errcheck/example

There are currently two flags: `-ignore` and `-ignorepkg`

The `-ignore` flag takes a regular expression of function names to ignore.
For example, you may wish to ignore common operations like Read and Write:

    errcheck -ignore '[rR]ead|[wW]rite' path/to/package

The `-ignorepkg` flag takes a comma-separated list of package import paths
to ignore. By default the `fmt` package is ignored, so you should include
it in your list if you want the default behavior:

    errcheck -ignorepkg 'fmt,encoding/binary' path/to/package

An example of using errcheck to check the go standard library packages:

    go list std | grep -v cmd | xargs -n 1 ./errcheck -ignore 'Close|[wW]rite.*|Flush|Seek|[rR]ead.*'> stdlibcheck

