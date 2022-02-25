<!-- [![github-actions](https://github.com/charlievieth/fastwalk/actions/workflows/go.yml/badge.svg)](https://github.com/charlievieth/fastwalk/actions) [![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg)](https://pkg.go.dev/github.com/charlievieth/fastwalk) -->

[![Test fastwalk on macOS](https://github.com/charlievieth/fastwalk/actions/workflows/macos.yml/badge.svg)](https://github.com/charlievieth/fastwalk/actions/workflows/macos.yml)
[![Test fastwalk on Linux](https://github.com/charlievieth/fastwalk/actions/workflows/linux.yml/badge.svg)](https://github.com/charlievieth/fastwalk/actions/workflows/linux.yml)
[![Test fastwalk on Windows](https://github.com/charlievieth/fastwalk/actions/workflows/windows.yml/badge.svg)](https://github.com/charlievieth/fastwalk/actions/workflows/windows.yml)
[![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg)](https://pkg.go.dev/github.com/charlievieth/fastwalk)

# fastwalk

Fast parallel directory traversal for Golang.

Package fastwalk provides a fast parallel version of [`filepath.WalkDir`](https://pkg.go.dev/io/fs#WalkDirFunc)
that is roughly \~4x faster on Linux and \~1.5x faster on macOS,
allocates 50% less memory, and requires 25% fewer memory allocations.

## Usage

Usage is the same as [`filepath.WalkDir`](https://pkg.go.dev/io/fs#WalkDirFunc)
only the [`filepath.WalkFunc`](https://pkg.go.dev/path/filepath@go1.17.7#WalkFunc)
needs to be safe for concurrent use.

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
