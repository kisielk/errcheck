errcheck
========

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

    go list std | grep -v cmd | xargs -n 1 errcheck -ignore 'Close|[wW]rite.*|Flush|Seek|[rR]ead.*'> stdlibcheck

Exit Codes
----------

errcheck returns 1 if any problems were found in the checked files.
It returns 2 on any if there were any other failures.

Editor Integration
==================

Emacs
-----
If you want to use errcheck together with compilation-mode, add
the following to your `.emacs` file to enable it to parse the
output and create hyperlinks to the relevant spots in your code:

```el
(add-to-list 'compilation-error-regexp-alist 'go-errcheck)
(add-to-list 'compilation-error-regexp-alist-alist
             '(go-errcheck "^\\(.+?\\):\\([[:digit:]]+\\):\\([[:digit:]]+\\) \t.+$" 1 2 3 1 1))
```

You can then use `M-x compile RET errcheck your/import/path` to
run errcheck from within Emacs.
