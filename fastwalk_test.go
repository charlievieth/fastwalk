// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fastwalk_test

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/charlievieth/utils/fastwalk"
)

func formatFileModes(m map[string]os.FileMode) string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	for _, k := range keys {
		fmt.Fprintf(&buf, "%-20s: %v\n", k, m[k])
	}
	return buf.String()
}

func writeFile(filename string, data interface{}, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	switch v := data.(type) {
	case []byte:
		_, err = f.Write(v)
	case string:
		_, err = f.WriteString(v)
	case io.Reader:
		_, err = io.Copy(f, v)
	default:
		f.Close()
		return &os.PathError{Op: "WriteFile", Path: filename,
			Err: fmt.Errorf("invalid data type: %T", data)}
	}
	if err1 := f.Close(); err1 != nil && err == nil {
		err = err1
	}
	return err
}

func symlink(t testing.TB, oldname, newname string) error {
	err := os.Symlink(oldname, newname)
	if err != nil {
		if writeErr := ioutil.WriteFile(newname, []byte(newname), 0644); writeErr == nil {
			// Couldn't create symlink, but could write the file.
			// Probably this filesystem doesn't support symlinks.
			// (Perhaps we are on an older Windows and not running as administrator.)
			t.Skipf("skipping because symlinks appear to be unsupported: %v", err)
		}
	}
	return err
}

func testFastWalkConf(t *testing.T, conf *fastwalk.Config, files map[string]string, callback func(path string, typ fastwalk.DirEntry) error, want map[string]os.FileMode) {
	tempdir, err := ioutil.TempDir("", "test-fast-walk")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempdir)

	symlinks := map[string]string{}
	for path, contents := range files {
		file := filepath.Join(tempdir, "/src", path)
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatal(err)
		}
		var err error
		if strings.HasPrefix(contents, "LINK:") {
			symlinks[file] = filepath.FromSlash(strings.TrimPrefix(contents, "LINK:"))
		} else {
			err = ioutil.WriteFile(file, []byte(contents), 0644)
		}
		if err != nil {
			t.Fatal(err)
		}
	}

	// Create symlinks after all other files. Otherwise, directory symlinks on
	// Windows are unusable (see https://golang.org/issue/39183).
	for file, dst := range symlinks {
		err = symlink(t, dst, file)
	}

	got := map[string]os.FileMode{}
	var mu sync.Mutex
	err = fastwalk.Walk(conf, tempdir, func(path string, de fastwalk.DirEntry) error {
		mu.Lock()
		defer mu.Unlock()
		if !strings.HasPrefix(path, tempdir) {
			t.Errorf("bogus prefix on %q, expect %q", path, tempdir)
		}
		key := filepath.ToSlash(strings.TrimPrefix(path, tempdir))
		if old, dup := got[key]; dup {
			t.Errorf("callback called twice for key %q: %v -> %v", key, old, de.Type())
		}
		got[key] = de.Type()
		return callback(path, de)
	})

	if err != nil {
		t.Fatalf("callback returned: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("walk mismatch.\n got:\n%v\nwant:\n%v", formatFileModes(got), formatFileModes(want))
	}
}

func testFastWalk(t *testing.T, files map[string]string, callback func(path string, typ fastwalk.DirEntry) error, want map[string]os.FileMode) {
	testFastWalkConf(t, nil, files, callback, want)
}

func TestFastWalk_Basic(t *testing.T) {
	testFastWalk(t, map[string]string{
		"foo/foo.go":   "one",
		"bar/bar.go":   "two",
		"skip/skip.go": "skip",
	},
		func(path string, typ fastwalk.DirEntry) error {
			return nil
		},
		map[string]os.FileMode{
			"":                  os.ModeDir,
			"/src":              os.ModeDir,
			"/src/bar":          os.ModeDir,
			"/src/bar/bar.go":   0,
			"/src/foo":          os.ModeDir,
			"/src/foo/foo.go":   0,
			"/src/skip":         os.ModeDir,
			"/src/skip/skip.go": 0,
		})
}

func TestFastWalk_LongFileName(t *testing.T) {
	longFileName := strings.Repeat("x", 255)

	testFastWalk(t, map[string]string{
		longFileName: "one",
	},
		func(path string, typ fastwalk.DirEntry) error {
			return nil
		},
		map[string]os.FileMode{
			"":                     os.ModeDir,
			"/src":                 os.ModeDir,
			"/src/" + longFileName: 0,
		},
	)
}

func TestFastWalk_Symlink(t *testing.T) {
	testFastWalk(t, map[string]string{
		"foo/foo.go":       "one",
		"bar/bar.go":       "LINK:../foo/foo.go",
		"symdir":           "LINK:foo",
		"broken/broken.go": "LINK:../nonexistent",
	},
		func(path string, typ fastwalk.DirEntry) error {
			return nil
		},
		map[string]os.FileMode{
			"":                      os.ModeDir,
			"/src":                  os.ModeDir,
			"/src/bar":              os.ModeDir,
			"/src/bar/bar.go":       os.ModeSymlink,
			"/src/foo":              os.ModeDir,
			"/src/foo/foo.go":       0,
			"/src/symdir":           os.ModeSymlink,
			"/src/broken":           os.ModeDir,
			"/src/broken/broken.go": os.ModeSymlink,
		})
}

func TestFastWalk_SkipDir(t *testing.T) {
	testFastWalk(t, map[string]string{
		"foo/foo.go":   "one",
		"bar/bar.go":   "two",
		"skip/skip.go": "skip",
	},
		func(path string, de fastwalk.DirEntry) error {
			typ := de.Type().Type()
			if typ == os.ModeDir && strings.HasSuffix(path, "skip") {
				return filepath.SkipDir
			}
			return nil
		},
		map[string]os.FileMode{
			"":                os.ModeDir,
			"/src":            os.ModeDir,
			"/src/bar":        os.ModeDir,
			"/src/bar/bar.go": 0,
			"/src/foo":        os.ModeDir,
			"/src/foo/foo.go": 0,
			"/src/skip":       os.ModeDir,
		})
}

func TestFastWalk_SkipFiles(t *testing.T) {
	// Directory iteration order is undefined, so there's no way to know
	// which file to expect until the walk happens. Rather than mess
	// with the test infrastructure, just mutate want.
	var mu sync.Mutex
	want := map[string]os.FileMode{
		"":              os.ModeDir,
		"/src":          os.ModeDir,
		"/src/zzz":      os.ModeDir,
		"/src/zzz/c.go": 0,
	}

	testFastWalk(t, map[string]string{
		"a_skipfiles.go": "a",
		"b_skipfiles.go": "b",
		"zzz/c.go":       "c",
	},
		func(path string, _ fastwalk.DirEntry) error {
			if strings.HasSuffix(path, "_skipfiles.go") {
				mu.Lock()
				defer mu.Unlock()
				want["/src/"+filepath.Base(path)] = 0
				return fastwalk.ErrSkipFiles
			}
			return nil
		},
		want)
	if len(want) != 5 {
		t.Errorf("saw too many files: wanted 5, got %v (%v)", len(want), want)
	}
}

func TestFastWalk_TraverseSymlink(t *testing.T) {
	testFastWalk(t, map[string]string{
		"foo/foo.go":   "one",
		"bar/bar.go":   "two",
		"skip/skip.go": "skip",
		"symdir":       "LINK:foo",
	},
		func(path string, de fastwalk.DirEntry) error {
			typ := de.Type().Type()
			if typ == os.ModeSymlink {
				return fastwalk.ErrTraverseLink
			}
			return nil
		},
		map[string]os.FileMode{
			"":                   os.ModeDir,
			"/src":               os.ModeDir,
			"/src/bar":           os.ModeDir,
			"/src/bar/bar.go":    0,
			"/src/foo":           os.ModeDir,
			"/src/foo/foo.go":    0,
			"/src/skip":          os.ModeDir,
			"/src/skip/skip.go":  0,
			"/src/symdir":        os.ModeSymlink,
			"/src/symdir/foo.go": 0,
		})
}

func TestFastWalk_TraverseSymlink_Follow(t *testing.T) {
	conf := fastwalk.Config{
		Follow: true,
	}
	testFastWalkConf(t, &conf, map[string]string{
		"foo/foo.go":   "one",
		"bar/bar.go":   "two",
		"skip/skip.go": "skip",
		"foo/symdir":   "LINK:foo",
		"bar/symdir":   "LINK:../foo",
	},
		func(path string, de fastwalk.DirEntry) error {
			typ := de.Type().Type()
			if typ == os.ModeSymlink {
				t.Errorf("unexpected symlink: %q", path)
			}
			return nil
		},
		map[string]os.FileMode{
			"":                  os.ModeDir,
			"/src":              os.ModeDir,
			"/src/bar":          os.ModeDir,
			"/src/bar/bar.go":   0,
			"/src/foo":          os.ModeDir,
			"/src/foo/foo.go":   0,
			"/src/skip":         os.ModeDir,
			"/src/skip/skip.go": 0,
		})
}

func TestFastWalk_FollowSymlinks(t *testing.T) {
	testFastWalk(t, map[string]string{
		"foo/foo.go":   "one",
		"bar/bar.go":   "two",
		"skip/skip.go": "skip",
		"bar/loop":     "LINK:../bar/",
	},
		fastwalk.FollowSymlinks(func(path string, de fastwalk.DirEntry) error {
			return nil
		}),
		map[string]os.FileMode{
			"":                  os.ModeDir,
			"/src":              os.ModeDir,
			"/src/bar":          os.ModeDir,
			"/src/bar/loop":     os.ModeSymlink,
			"/src/bar/bar.go":   0,
			"/src/foo":          os.ModeDir,
			"/src/foo/foo.go":   0,
			"/src/skip":         os.ModeDir,
			"/src/skip/skip.go": 0,
		})
}

func TestFastWalk_SymlinkLoop(t *testing.T) {
	tempdir, err := ioutil.TempDir("", "test-fast-walk")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempdir)

	if err := writeFile(tempdir+"/src/foo.go", "hello", 0644); err != nil {
		t.Fatal(err)
	}
	if err := symlink(t, "../src", tempdir+"/src/loop"); err != nil {
		t.Fatal(err)
	}

	conf := fastwalk.Config{
		Follow: true,
	}
	var walked int32
	err = fastwalk.Walk(&conf, tempdir, func(path string, de fastwalk.DirEntry) error {
		if n := atomic.AddInt32(&walked, 1); n > 20 {
			return fmt.Errorf("symlink loop: %d", n)
		}
		if de.Type()&os.ModeSymlink != 0 {
			return fastwalk.ErrTraverseLink
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFastWalk_Error(t *testing.T) {
	tmp := t.TempDir()
	for _, child := range []string{
		"foo/foo.go",
		"bar/bar.go",
		"skip/skip.go",
	} {
		if err := writeFile(filepath.Join(tmp, child), child, 0644); err != nil {
			t.Fatal(err)
		}
	}

	exp := errors.New("expected")
	err := fastwalk.Walk(nil, tmp, func(_ string, _ fastwalk.DirEntry) error {
		return exp
	})
	if !errors.Is(err, exp) {
		t.Errorf("want error: %#v got: %#v", exp, err)
	}
}

func TestFastWalk_ErrNotExist(t *testing.T) {
	tmp := t.TempDir()
	if err := os.Remove(tmp); err != nil {
		t.Fatal(err)
	}
	err := fastwalk.Walk(nil, tmp, func(_ string, _ fastwalk.DirEntry) error {
		return nil
	})
	if !os.IsNotExist(err) {
		t.Fatalf("os.IsNotExist(%+v) = false want: true", err)
	}
}

func diffFileMaps(t testing.TB, got, want map[string]os.FileMode) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Log("cannot diff files:", err)
	}
	tempdir, err := os.MkdirTemp(t.TempDir(), "diff-*")
	if err != nil {
		t.Fatal(err)
	}
	gotName := filepath.Join(tempdir, "/got")
	wantName := filepath.Join(tempdir, "/want")
	if err := writeFile(gotName, formatFileModes(got), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(wantName, formatFileModes(want), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("git", "diff", "--no-index", "--color=always", "want", "got")
	cmd.Dir = tempdir
	out, err := cmd.CombinedOutput()
	out = bytes.TrimSpace(out)
	if err != nil && bytes.Contains(out, []byte("error:")) {
		t.Fatalf("error running command: %q: %v\n### Output\n%s\n####\n",
			cmd.Args, err, out)
		return
	}
	t.Logf("## Diff:\n%s\n", bytes.TrimSpace(out))
}

func TestFastWalk_ErrPermission(t *testing.T) {
	tempdir := t.TempDir()
	want := map[string]os.FileMode{
		"":     os.ModeDir,
		"/bad": os.ModeDir,
	}
	for i := 0; i < runtime.NumCPU()*4; i++ {
		dir := fmt.Sprintf("/d%03d", i)
		name := fmt.Sprintf("%s/f%03d.txt", dir, i)
		if err := writeFile(filepath.Join(tempdir, name), "data", 0644); err != nil {
			t.Fatal(err)
		}
		want[name] = 0
		want[filepath.Dir(name)] = os.ModeDir
	}

	filename := filepath.Join(tempdir, "/bad/bad.txt")
	if err := writeFile(filename, "data", 0644); err != nil {
		t.Fatal(err)
	}
	// Make the directory unreadable
	dirname := filepath.Dir(filename)
	if err := os.Chmod(dirname, 0355); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Remove(filename); err != nil {
			t.Error(err)
		}
		if err := os.Remove(dirname); err != nil {
			t.Error(err)
		}
	})

	got := map[string]os.FileMode{}
	var mu sync.Mutex
	err := fastwalk.Walk(nil, tempdir, func(path string, de fastwalk.DirEntry) error {
		mu.Lock()
		defer mu.Unlock()
		if !strings.HasPrefix(path, tempdir) {
			t.Errorf("bogus prefix on %q, expect %q", path, tempdir)
		}
		key := filepath.ToSlash(strings.TrimPrefix(path, tempdir))
		if old, dup := got[key]; dup {
			t.Errorf("callback called twice for key %q: %v -> %v", key, old, de.Type())
		}
		got[key] = de.Type()
		return nil
	})
	if err != nil {
		t.Error("Walk:", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("walk mismatch.\n got:\n%v\nwant:\n%v", formatFileModes(got), formatFileModes(want))
		diffFileMaps(t, got, want)
	}

	// err := fastwalk.Walk(nil, tmp, func(_ string, _ fastwalk.DirEntry) error {
	// 	return nil
	// })

	// if _, err := os.Lstat(filename); err == nil {
	// 	t.Fatal(err)
	// }

	// TODO: `mkdir bad && chmod 0375 bad && rmdir bad`
	// t.Skip("TODO: implement test")
}

var benchDir = flag.String("benchdir", runtime.GOROOT(), "The directory to scan for BenchmarkFastWalk")

func benchmarkFastWalk(b *testing.B, conf *fastwalk.Config) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		err := fastwalk.Walk(conf, *benchDir, func(path string, de fastwalk.DirEntry) error { return nil })
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFastWalk(b *testing.B) {
	benchmarkFastWalk(b, nil)
}

func BenchmarkFastWalkFollow(b *testing.B) {
	benchmarkFastWalk(b, &fastwalk.Config{
		Follow: true,
	})
}
