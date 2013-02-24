errcheck
=========

errcheck is a program for checking for unchecked errors in go programs.

Install
-------

    go get github.com/kisielk/errcheck

Note that errcheck depends on the go/types package which is currently only available in go tip.

Use
---

For basic usage, just give the package path of interest as the first
argument:

    errcheck github.com/kisielk/errcheck/example

The output format is incomplete.
