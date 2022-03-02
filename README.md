<!-- [![github-actions](https://github.com/charlievieth/fastwalk/actions/workflows/go.yml/badge.svg)](https://github.com/charlievieth/fastwalk/actions) [![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg)](https://pkg.go.dev/github.com/charlievieth/fastwalk) -->

[![Test fastwalk on macOS](https://github.com/charlievieth/fastwalk/actions/workflows/macos.yml/badge.svg)](https://github.com/charlievieth/fastwalk/actions/workflows/macos.yml)
[![Test fastwalk on Linux](https://github.com/charlievieth/fastwalk/actions/workflows/linux.yml/badge.svg)](https://github.com/charlievieth/fastwalk/actions/workflows/linux.yml)
[![Test fastwalk on Windows](https://github.com/charlievieth/fastwalk/actions/workflows/windows.yml/badge.svg)](https://github.com/charlievieth/fastwalk/actions/workflows/windows.yml)
[![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg)](https://pkg.go.dev/github.com/charlievieth/fastwalk)

# fastwalk

Fast parallel directory traversal for Golang.

Package fastwalk provides a fast parallel version of [`filepath.WalkDir`](https://pkg.go.dev/io/fs#WalkDirFunc)
that is \~4x faster on Linux, \~1.5x faster on macOS, allocates 50% less memory,
and requires 25% fewer memory allocations.

<!-- TODO: mention EntryFilter -->

## Usage

Usage is the same as [`filepath.WalkDir`](https://pkg.go.dev/io/fs#WalkDirFunc),
but the [`walkFn`](https://pkg.go.dev/path/filepath@go1.17.7#WalkFunc)
argument to [`fastwalk.Walk`](https://pkg.go.dev/github.com/charlievieth/fastwalk#Walk)
must be safe for concurrent use.

<!-- TODO: this example is large move it to an examples folder -->

The below example recursively walks a directory following symbolic links and
prints the number of lines found in each file (or symbolic link that references
a file) to stdout:
```go
import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/charlievieth/fastwalk"
)

var newLine = []byte{'\n'}

// countLinesInFile returns the number of newlines ('\n') in file name.
func countLinesInFile(name string) (int64, error) {
	f, err := os.Open(name)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	buf := make([]byte, 96*1024)
	var lines int64
	for {
		n, e := f.Read(buf)
		if n > 0 {
			lines += int64(bytes.Count(buf[:n], newLine))
		}
		if e != nil {
			if e != io.EOF {
				err = e
			}
			break
		}
	}
	return lines, err
}

// LineCount recursively walks directory root printing the number of lines in
// file encountered.
func LineCount(root string) error {
	countLinesWalkFn := func(path string, d fs.DirEntry, err error) error {
		// We wrap this with fastwalk.IgnorePermissionErrors so we know the
		// error is not a permission error (common when walking outside a users
		// home directory) and is likely something worse so we should return it
		// and abort the walk.
		//
		// A common error here is "too many open files", which can occur if the
		// walkFn opens, but does not close, files.
		if err != nil {
			return err
		}

		// If the entry is a symbolic link get the type of file that
		// it references.
		typ := d.Type()
		if typ&fs.ModeSymlink != 0 {
			if fi, err := fastwalk.StatDirEntry(path, d); err == nil {
				typ = fi.Mode().Type()
			}
		}

		if typ.IsRegular() {
			lines, err := countLinesInFile(path)
			if err == nil {
				fmt.Printf("%8d %s\n", lines, path)
			} else {
				// Print but do not return the error.
				fmt.Fprintf(os.Stderr, "%s: %s\n", path, err)
			}
		}
		return nil
	}

	// Ignore permission errors traversing directories.
	//
	// Note: this only ignores permission errors when traversing directories.
	// Permission errors may still be encountered when accessing files.
	walkFn := fastwalk.IgnorePermissionErrors(countLinesWalkFn)

	// Safely follow symbolic links. This can also be achieved by setting
	// fastwalk.Config.Follow to true.
	walkFn = fastwalk.FollowSymlinks(walkFn)

	// If Walk is called with a nil Config the DefaultConfig is used.
	if err := fastwalk.Walk(nil, root, walkFn); err != nil {
                return fmt.Errorf("walking directory %s: %v", root, err)
	}
        return nil
}
```

## Benchmarks

Benchmarks were created using `go1.17.6` and can be generated with the `bench_comp` make target:
```sh
$ make bench_comp
```

### Darwin

**Hardware:**
```
goos: darwin
goarch: arm64
cpu: Apple M1 Max
```

#### [`filepath.WalkDir`](https://pkg.go.dev/path/filepath@go1.17.7#WalkDir) vs. [`fastwalk.Walk()`](https://pkg.go.dev/github.com/charlievieth/fastwalk#Walk):
```
              filepath       fastwalk       delta
time/op       28.9ms ± 1%    18.0ms ± 2%    -37.88%
alloc/op      4.33MB ± 0%    2.14MB ± 0%    -50.67%
allocs/op     50.9k ± 0%     37.7k ± 0%     -26.01%
```

#### [`godirwalk.Walk()`](https://pkg.go.dev/github.com/karrick/godirwalk@v1.16.1#Walk) vs. [`fastwalk.Walk()`](https://pkg.go.dev/github.com/charlievieth/fastwalk#Walk):
```
              godirwalk      fastwalk       delta
time/op       58.5ms ± 3%    18.0ms ± 2%    -69.30%
alloc/op      25.3MB ± 0%    2.1MB ± 0%     -91.55%
allocs/op     57.6k ± 0%     37.7k ± 0%     -34.59%
```

### Linux

**Hardware:**
```
goos: linux
goarch: amd64
cpu: Intel(R) Core(TM) i9-9900K CPU @ 3.60GHz
drive: Samsung SSD 970 PRO 1TB
```

#### [`filepath.WalkDir`](https://pkg.go.dev/path/filepath@go1.17.7#WalkDir) vs. [`fastwalk.Walk()`](https://pkg.go.dev/github.com/charlievieth/fastwalk#Walk):

```
              filepath       fastwalk       delta
time/op       10.1ms ± 2%    2.8ms ± 2%     -72.83%
alloc/op      2.44MB ± 0%    1.70MB ± 0%    -30.46%
allocs/op     47.2k ± 0%     36.9k ± 0%     -21.80%
```

#### [`godirwalk.Walk()`](https://pkg.go.dev/github.com/karrick/godirwalk@v1.16.1#Walk) vs. [`fastwalk.Walk()`](https://pkg.go.dev/github.com/charlievieth/fastwalk#Walk):

```
              filepath       fastwalk       delta
time/op       13.7ms ±16%    2.8ms ± 2%     -79.88%
alloc/op      7.48MB ± 0%    1.70MB ± 0%    -77.34%
allocs/op     53.8k ± 0%     36.9k ± 0%     -31.38%
```

## Darwin: getdirentries64

The `nogetdirentries` build tag can be used to prevent `fastwalk` from using
and linking to the non-public `__getdirentries64` syscall. This is required
if an app using `fastwalk` is to be distributed via Apple's App Store (see
https://github.com/golang/go/issues/30933 for more details). When using
`__getdirentries64` is disabled, `fastwalk` will use `readdir_r` instead,
which is what the Go standard library uses for
[`os.ReadDir`](https://pkg.go.dev/os#ReadDir) and is about \~10% slower than
`__getdirentries64`
([benchmarks](https://github.com/charlievieth/fastwalk/blob/2e6a1b8a1ce88e578279e6e631b2129f7144ec87/fastwalk_darwin_test.go#L19-L57)).

Example of how to build and test that your program is not linked to `__getdirentries64`:
```sh
# NOTE: the following only applies to darwin (aka macOS)

# Build binary that imports fastwalk without linking to __getdirentries64.
$ go build -tags nogetdirentries -o YOUR_BINARY
# Test that __getdirentries64 is not linked.
$ otool -dyld_info YOUR_BINARY | grep -F getdirentries64
```
