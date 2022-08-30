package fastwalk_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/charlievieth/fastwalk"
)

func TestIgnoreDuplicateDirs(t *testing.T) {
	tempdir, err := os.MkdirTemp("", "test-fast-walk")
	if err != nil {
		t.Fatal(err)
	}
	// on macOS the tempdir is a symlink
	tempdir, err = filepath.EvalSymlinks(tempdir)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupOrLogTempDir(t, tempdir)

	files := map[string]string{
		"bar/bar.go":  "one",
		"foo/foo.go":  "two",
		"skip/baz.go": "three", // we skip "skip", but visit "baz.go" via "symdir"
		"symdir":      "LINK:skip",
		"bar/symdir":  "LINK:../foo/",
		"bar/loop":    "LINK:../bar/", // symlink loop
	}
	testCreateFiles(t, tempdir, files)

	want := map[string]os.FileMode{
		"":                   os.ModeDir,
		"/src":               os.ModeDir,
		"/src/bar":           os.ModeDir,
		"/src/bar/bar.go":    0,
		"/src/bar/symdir":    os.ModeSymlink,
		"/src/bar/loop":      os.ModeSymlink,
		"/src/foo":           os.ModeDir,
		"/src/foo/foo.go":    0,
		"/src/symdir":        os.ModeSymlink,
		"/src/symdir/baz.go": 0,
		"/src/skip":          os.ModeDir,
	}

	runTest := func(t *testing.T, conf *fastwalk.Config) {
		var mu sync.Mutex
		got := make(map[string]os.FileMode)
		walkFn := fastwalk.IgnoreDuplicateDirs(func(path string, de fs.DirEntry, err error) error {
			requireNoError(t, err)
			if err != nil {
				return err
			}

			// Resolve links for regular files since we don't know which directory
			// or link we traversed to visit them. Exclude "baz.go" because we want
			// to test that we visited it through it's link.
			if de.Type().IsRegular() && de.Name() != "baz.go" {
				realpath, err := filepath.EvalSymlinks(path)
				if err != nil {
					t.Error(err)
					return err
				}
				path = realpath
			}
			if !strings.HasPrefix(path, tempdir) {
				t.Errorf("Path %q not a child of TMPDIR %q", path, tempdir)
				return errors.New("abort")
			}
			key := filepath.ToSlash(strings.TrimPrefix(path, tempdir))

			mu.Lock()
			defer mu.Unlock()
			got[key] = de.Type().Type()

			if de.Name() == "skip" {
				return filepath.SkipDir
			}
			return nil
		})
		if err := fastwalk.Walk(conf, tempdir, walkFn); err != nil {
			t.Error("fastwalk:", err)
		}
		if !reflect.DeepEqual(want, got) {
			t.Errorf("walk mismatch.\n got:\n%v\nwant:\n%v", formatFileModes(got), formatFileModes(want))
			diffFileModes(t, got, want)
		}
	}

	t.Run("NoFollow", func(t *testing.T) {
		runTest(t, &fastwalk.Config{Follow: false})
	})

	// Test that setting Follow to true has no impact on the behavior
	t.Run("Follow", func(t *testing.T) {
		runTest(t, &fastwalk.Config{Follow: true})
	})

	t.Run("Error", func(t *testing.T) {
		tempdir := t.TempDir()
		if err := os.WriteFile(tempdir+"/error_test", []byte("error"), 0644); err != nil {
			t.Fatal(err)
		}
		want := errors.New("my error")
		var callCount int32
		walkFn := fastwalk.IgnoreDuplicateDirs(func(path string, de fs.DirEntry, err error) error {
			atomic.AddInt32(&callCount, 1)
			return want
		})
		err := fastwalk.Walk(nil, tempdir, walkFn)
		if !errors.Is(err, want) {
			t.Errorf("Error: want: %v got: %v", want, err)
		}

	})
}

func TestIgnoreDuplicateFiles(t *testing.T) {
	tempdir := t.TempDir()
	files := map[string]string{
		"foo/foo.go":       "one",
		"bar/bar.go":       "LINK:../foo/foo.go",
		"bar/baz.go":       "two",
		"broken/broken.go": "LINK:../nonexistent",
		"bar/loop":         "LINK:../bar/", // symlink loop
		"file.go":          "three",

		// Use multiple symdirs to increase the chance that one
		// of these and not "foo" is followed first.
		"symdir1": "LINK:foo",
		"symdir2": "LINK:foo",
		"symdir3": "LINK:foo",
		"symdir4": "LINK:foo",
	}
	if runtime.GOOS == "windows" {
		delete(files, "broken/broken.go")
	}
	testCreateFiles(t, tempdir, files)

	var expectedContents []string
	for _, contents := range files {
		if !strings.HasPrefix(contents, "LINK:") {
			expectedContents = append(expectedContents, contents)
		}
	}
	sort.Strings(expectedContents)

	var (
		mu       sync.Mutex
		seen     []os.FileInfo
		contents []string
	)
	walkFn := fastwalk.IgnoreDuplicateFiles(func(path string, de fs.DirEntry, err error) error {
		requireNoError(t, err)
		fi1, err := fastwalk.StatDirEntry(path, de)
		if err != nil {
			t.Error(err)
			return err
		}
		mu.Lock()
		defer mu.Unlock()
		for _, fi2 := range seen {
			if os.SameFile(fi1, fi2) {
				t.Errorf("Visited file twice: %q (%s) and %q (%s)",
					path, fi1.Mode(), fi2.Name(), fi2.Mode())
			}
		}
		seen = append(seen, fi1)
		if fi1.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			contents = append(contents, string(data))
		}
		return nil
	})
	if err := fastwalk.Walk(nil, tempdir, walkFn); err != nil {
		t.Fatal(err)
	}

	sort.Strings(contents)
	if !reflect.DeepEqual(expectedContents, contents) {
		t.Errorf("File contents want: %q got: %q", expectedContents, contents)
	}
}

func TestIgnorePermissionErrors(t *testing.T) {
	var called bool
	fn := fastwalk.IgnorePermissionErrors(func(path string, _ fs.DirEntry, err error) error {
		called = true
		if err != nil {
			t.Fatal(err)
		}
		return nil
	})

	t.Run("PermissionError", func(t *testing.T) {
		err := fn("", nil, &os.PathError{Op: "open", Path: "foo.go", Err: os.ErrPermission})
		if err != nil {
			t.Fatal(err)
		}
		if called {
			t.Fatal("walkFn should not have been called with os.ErrPermission")
		}
	})

	t.Run("NilError", func(t *testing.T) {
		called = false
		if err := fn("", nil, nil); err != nil {
			t.Fatal(err)
		}
		if !called {
			t.Fatal("walkFn should have been called with nil error")
		}
	})

	t.Run("OtherError", func(t *testing.T) {
		fn := fastwalk.IgnorePermissionErrors(func(path string, _ fs.DirEntry, err error) error {
			return err
		})
		want := &os.PathError{Op: "open", Path: "foo.go", Err: os.ErrExist}
		if got := fn("", nil, want); got != want {
			t.Fatalf("want error: %v got: %v", want, got)
		}
	})
}
