package fastwalk_test

import (
	"io/fs"
	"os"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/charlievieth/fastwalk"
)

// WARN: this benchmark is pretty useless
func BenchmarkAdapterOverhead(b *testing.B) {
	b.Run("Baseline", func(b *testing.B) {
		noop := func(_ string, _ fs.DirEntry, _ error) error {
			return nil
		}
		for i := 0; i < b.N; i++ {
			noop("/", nil, nil)
		}
	})
	b.Run("Adapter", func(b *testing.B) {
		noop := func(_ string, _ fs.DirEntry, _ error) error {
			return nil
		}
		fn := fastwalk.IgnorePermissionErrors(noop)
		for i := 0; i < b.N; i++ {
			fn("/", nil, nil)
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
