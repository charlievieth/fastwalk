package fastwalk_test

import (
	"bytes"
	"crypto/md5"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"math"
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
		if writeErr := os.WriteFile(newname, []byte(newname), 0644); writeErr == nil {
			// Couldn't create symlink, but could write the file.
			// Probably this filesystem doesn't support symlinks.
			// (Perhaps we are on an older Windows and not running as administrator.)
			t.Skipf("skipping because symlinks appear to be unsupported: %v", err)
		}
	}
	return err
}

func cleanupOrLogTempDir(t *testing.T, tempdir string) {
	if e := recover(); e != nil {
		t.Log("TMPDIR:", filepath.ToSlash(tempdir))
		t.Fatal(e)
	}
	if t.Failed() {
		t.Log("TMPDIR:", filepath.ToSlash(tempdir))
	} else {
		os.RemoveAll(tempdir)
	}
}

func testCreateFiles(t *testing.T, tempdir string, files map[string]string) {
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
			err = os.WriteFile(file, []byte(contents), 0644)
		}
		if err != nil {
			t.Fatal(err)
		}
	}

	// Create symlinks after all other files. Otherwise, directory symlinks on
	// Windows are unusable (see https://golang.org/issue/39183).
	for file, dst := range symlinks {
		if err := symlink(t, dst, file); err != nil {
			t.Fatal(err)
		}
	}
}

func testFastWalkConf(t *testing.T, conf *fastwalk.Config, files map[string]string,
	callback fs.WalkDirFunc, want map[string]os.FileMode) {

	tempdir, err := os.MkdirTemp("", "test-fast-walk")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupOrLogTempDir(t, tempdir)

	testCreateFiles(t, tempdir, files)

	got := map[string]os.FileMode{}
	var mu sync.Mutex
	err = fastwalk.Walk(conf, tempdir, func(path string, de fs.DirEntry, err error) error {
		if de == nil {
			t.Errorf("nil fs.DirEntry on %q", path)
			return nil
		}
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
		return callback(path, de, err)
	})

	if err != nil {
		t.Fatalf("callback returned: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("walk mismatch.\n got:\n%v\nwant:\n%v", formatFileModes(got), formatFileModes(want))
		diffFileModes(t, got, want)
	}
}

func testFastWalk(t *testing.T, files map[string]string,
	callback fs.WalkDirFunc, want map[string]os.FileMode) {

	testFastWalkConf(t, nil, files, callback, want)
}

func requireNoError(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Error("WalkDirFunc called with error:", err)
		panic(err)
	}
}

func TestFastWalk_Basic(t *testing.T) {
	testFastWalk(t, map[string]string{
		"foo/foo.go":   "one",
		"bar/bar.go":   "two",
		"skip/skip.go": "skip",
	},
		func(path string, typ fs.DirEntry, err error) error {
			requireNoError(t, err)
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

func maxFileNameLength(t testing.TB) int {
	tmp := t.TempDir()
	long := strings.Repeat("a", 8192)

	// Returns if n is an invalid file name length
	invalidLength := func(n int) bool {
		path := filepath.Join(tmp, long[:n])
		err := os.WriteFile(path, []byte("1"), 0644)
		if err == nil {
			os.Remove(path)
		}
		return err != nil
	}

	// Use a binary search to find the max filename length (+1)
	n := sort.Search(8192, invalidLength)
	if n <= 1 {
		t.Fatal("Failed to find the max filename length:", n)
	}
	max := n - 1
	if invalidLength(max) {
		t.Fatal("Failed to find the max filename length:", n)
	}
	return max
}

// This test identified a "checkptr: converted pointer straddles multiple allocations"
// error on darwin when getdirentries64 was used with the race-detector enabled.
func TestFastWalk_LongFileName(t *testing.T) {
	// Test is slow since we need to find the longest allowed filename
	t.Parallel()

	maxNameLen := maxFileNameLength(t)
	if maxNameLen > 255 {
		maxNameLen = 255
	}
	want := map[string]os.FileMode{
		"":     os.ModeDir,
		"/src": os.ModeDir,
	}
	files := make(map[string]string)
	// This triggers with only one sub-directory but use 2 just to be sure.
	for r := 'a'; r <= 'b'; r++ {
		s := string(r)
		name := s + "/" + strings.Repeat(s, maxNameLen)
		for i := len("_/") + 1; i <= len(name); i++ {
			files[name[:i]] = "1"
			want["/src/"+name[:i]] = 0
		}
		want["/src/"+s] = os.ModeDir
	}
	testFastWalk(t, files,
		func(path string, typ fs.DirEntry, err error) error {
			requireNoError(t, err)
			return nil
		},
		want,
	)
}

func maxPathLength(t testing.TB) (root string, pathMax int) {
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	switch len(tmp) % 4 {
	case 0:
	case 1:
		// Can't just add 1 "/" so add 5 ("/aaaa")
		tmp = filepath.Join(tmp, "/aaaa")
	case 2:
		tmp = filepath.Join(tmp, "/a")
	case 3:
		tmp = filepath.Join(tmp, "/aa")
	}
	base := tmp

	// Returns if n is an invalid file name length
	var longestPath string
	invalidPathLength := func(n int) bool {
		m := n - len(tmp)
		if m <= 0 {
			return false
		}
		var w strings.Builder
		w.Grow(n + 1)
		w.WriteString(base)
		elem := "/" + strings.Repeat("a", 127) // path element
		for w.Len() < n-len(elem) {
			w.WriteString(elem)
		}
		for w.Len() < n {
			w.WriteByte('b')
		}
		path := w.String()
		if len(path) != n {
			t.Fatalf("invalid PATH length: %d want: %d", len(path), n)
		}
		err := os.MkdirAll(path, 0755)
		if err == nil {
			// Don't remove directories on success since it's slow
			// and we'll use them again as the path length increases.
			longestPath = path
		}
		return err != nil
	}

	// Use a binary search to find the max path length (+1)
	n := sort.Search(16*1024, invalidPathLength)
	if n <= 1 {
		t.Fatal("Failed to find the max path length:", n)
	}
	pathMax = n - 1
	if invalidPathLength(pathMax) {
		t.Fatal("Failed to find the max path length:", n)
	}
	// Make sure longestPath exists
	if _, err := os.Stat(longestPath); err != nil {
		t.Fatalf("Invalid longest path (%q): %v", longestPath, err)
	}

	// Create directories under the tmp/root dir: /{TMP}/{b..z}/{LONGEST_PATH}
	root = filepath.Dir(tmp)
	name := filepath.Base(tmp)
	long := strings.TrimPrefix(longestPath, tmp)
	end := 'z'
	if testing.Short() {
		end = 'e'
	}
	for r := 'b'; r <= end; r++ {
		newBase := strings.Repeat(string(r), len(name))
		if err := os.MkdirAll(filepath.Join(root, newBase, long), 0755); err != nil {
			t.Fatal(err)
		}
	}
	return root, pathMax
}

// Test that we can handle PATH_MAX. This is mostly for the Unix tests
// where we pass a buffer to ReadDirect (often getdents64(2)).
func TestFastWalk_LongPath(t *testing.T) {
	// Test is slow since we need to find the longest allowed file path
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("test not needed on Windows")
	}

	root, pathMax := maxPathLength(t)
	t.Log("PATH_MAX:", pathMax)

	var want []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		want = append(want, filepath.Clean(path))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	var mu sync.Mutex
	err = fastwalk.Walk(nil, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		mu.Lock()
		got = append(got, filepath.Clean(path))
		mu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	sort.Strings(want)
	sort.Strings(got)

	if !reflect.DeepEqual(want, got) {
		// Don't print the delta here since it might be very large. Instead
		// write it to two temp files in a directory that is not removed on
		// test exit so that the user can compare them themselves.
		tempdir, err := os.MkdirTemp("", "fastwalk-test-*")
		if err != nil {
			t.Error(err)
		}
		if err := writeFile(tempdir+"/want.txt", strings.Join(want, "\n"), 0666); err != nil {
			t.Error(err)
		}
		if err := writeFile(tempdir+"/got.txt", strings.Join(got, "\n"), 0666); err != nil {
			t.Error(err)
		}
		t.Fatalf("Output does not match: see the files in: %q", tempdir)
	}
}

func TestFastWalk_Symlink(t *testing.T) {
	testFastWalk(t, map[string]string{
		"foo/foo.go":       "one",
		"bar/bar.go":       "LINK:../foo/foo.go",
		"symdir":           "LINK:foo",
		"broken/broken.go": "LINK:../nonexistent",
	},
		func(path string, typ fs.DirEntry, err error) error {
			requireNoError(t, err)
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

// Test that the fs.DirEntry passed to WalkFunc is always a fastwalk.DirEntry.
func TestFastWalk_DirEntryType(t *testing.T) {
	testFastWalk(t, map[string]string{
		"foo/foo.go":       "one",
		"bar/bar.go":       "LINK:../foo/foo.go",
		"symdir":           "LINK:foo",
		"broken/broken.go": "LINK:../nonexistent",
	},
		func(path string, de fs.DirEntry, err error) error {
			requireNoError(t, err)
			if _, ok := de.(fastwalk.DirEntry); !ok {
				t.Errorf("%q: not a fastwalk.DirEntry: %T", path, de)
			}
			if de.Type() != de.Type().Type() {
				t.Errorf("%s: type mismatch got: %q want: %q",
					path, de.Type(), de.Type().Type())
			}
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
	test := func(t *testing.T, mode fastwalk.SortMode) {
		conf := fastwalk.DefaultConfig.Copy()
		conf.Sort = mode
		testFastWalkConf(t, conf, map[string]string{
			"foo/foo.go":   "one",
			"bar/bar.go":   "two",
			"skip/skip.go": "skip",
		},
			func(path string, de fs.DirEntry, err error) error {
				requireNoError(t, err)
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

	// Test that sorting respects fastwalk.ErrSkipFiles
	for _, mode := range []fastwalk.SortMode{
		fastwalk.SortNone,
		fastwalk.SortLexical,
		fastwalk.SortDirsFirst,
		fastwalk.SortFilesFirst,
	} {
		t.Run(mode.String(), func(t *testing.T) {
			test(t, mode)
		})
	}
}

func TestFastWalk_SkipFiles(t *testing.T) {
	mapKeys := func(m map[string]os.FileMode) []string {
		a := make([]string, 0, len(m))
		for k := range m {
			a = append(a, k)
		}
		return a
	}

	test := func(t *testing.T, mode fastwalk.SortMode) {
		// Directory iteration order is undefined, so there's no way to know
		// which file to expect until the walk happens. Rather than mess
		// with the test infrastructure, just mutate want.
		want := map[string]os.FileMode{
			"":              os.ModeDir,
			"/src":          os.ModeDir,
			"/src/zzz":      os.ModeDir,
			"/src/zzz/c.go": 0,
		}
		conf := fastwalk.DefaultConfig.Copy()
		conf.Sort = mode
		var mu sync.Mutex
		testFastWalkConf(t, conf, map[string]string{
			"a_skipfiles.go": "a",
			"b_skipfiles.go": "b",
			"zzz/c.go":       "c",
		},
			func(path string, _ fs.DirEntry, err error) error {
				requireNoError(t, err)
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
			t.Errorf("invalid number of files visited: wanted 5, got %v (%q)",
				len(want), mapKeys(want))
		}
	}

	// Test that sorting respects fastwalk.ErrSkipFiles
	for _, mode := range []fastwalk.SortMode{
		fastwalk.SortNone,
		fastwalk.SortLexical,
		fastwalk.SortDirsFirst,
		fastwalk.SortFilesFirst,
	} {
		t.Run(mode.String(), func(t *testing.T) {
			test(t, mode)
		})
	}
}

func TestFastWalk_TraverseSymlink(t *testing.T) {
	testFastWalk(t, map[string]string{
		"foo/foo.go": "one",
		"bar/bar.go": "two",
		"symdir":     "LINK:foo",
	},
		func(path string, de fs.DirEntry, err error) error {
			requireNoError(t, err)
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
			"/src/symdir":        os.ModeSymlink,
			"/src/symdir/foo.go": 0,
		})
}

func TestFastWalk_Follow(t *testing.T) {
	subTests := []struct {
		Name   string
		OnLink func(path string, d fs.DirEntry) error
	}{
		// Test that the walk func does *not* need to return
		// ErrTraverseLink for links to be followed.
		{
			Name:   "Default",
			OnLink: func(path string, d fs.DirEntry) error { return nil },
		},

		// Test that returning ErrTraverseLink does not interfere
		// with the Follow logic.
		{
			Name: "ErrTraverseLink",
			OnLink: func(path string, d fs.DirEntry) error {
				if d.Type()&os.ModeSymlink != 0 {
					if fi, err := fastwalk.StatDirEntry(path, d); err == nil && fi.IsDir() {
						return fastwalk.ErrTraverseLink
					}
				}
				return nil
			},
		},
	}
	for _, x := range subTests {
		t.Run(x.Name, func(t *testing.T) {
			conf := fastwalk.Config{
				Follow: true,
			}
			testFastWalkConf(t, &conf, map[string]string{
				"foo/foo.go":  "one",
				"bar/bar.go":  "two",
				"foo/symlink": "LINK:foo.go",
				"bar/symdir":  "LINK:../foo/",
				"bar/link1":   "LINK:../foo/",
			},
				func(path string, de fs.DirEntry, err error) error {
					requireNoError(t, err)
					if err != nil {
						return err
					}
					if de.Type()&os.ModeSymlink != 0 {
						return x.OnLink(path, de)
					}
					return nil
				},
				map[string]os.FileMode{
					"":                        os.ModeDir,
					"/src":                    os.ModeDir,
					"/src/bar":                os.ModeDir,
					"/src/bar/bar.go":         0,
					"/src/bar/link1":          os.ModeSymlink,
					"/src/bar/link1/foo.go":   0,
					"/src/bar/link1/symlink":  os.ModeSymlink,
					"/src/bar/symdir":         os.ModeSymlink,
					"/src/bar/symdir/foo.go":  0,
					"/src/bar/symdir/symlink": os.ModeSymlink,
					"/src/foo":                os.ModeDir,
					"/src/foo/foo.go":         0,
					"/src/foo/symlink":        os.ModeSymlink,
				})
		})
	}
}

func TestFastWalk_Follow_SkipDir(t *testing.T) {
	conf := fastwalk.Config{
		Follow: true,
	}
	testFastWalkConf(t, &conf, map[string]string{
		".dot/baz.go": "one",
		"bar/bar.go":  "three",
		"bar/dot":     "LINK:../.dot/",
		"bar/symdir":  "LINK:../foo/",
		"foo/foo.go":  "two",
		"foo/symlink": "LINK:foo.go",
	},
		func(path string, de fs.DirEntry, err error) error {
			requireNoError(t, err)
			if err != nil {
				return err
			}
			if strings.HasPrefix(de.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		},
		map[string]os.FileMode{
			"":                        os.ModeDir,
			"/src":                    os.ModeDir,
			"/src/.dot":               os.ModeDir,
			"/src/bar":                os.ModeDir,
			"/src/bar/bar.go":         0,
			"/src/bar/dot":            os.ModeSymlink,
			"/src/bar/dot/baz.go":     0,
			"/src/bar/symdir":         os.ModeSymlink,
			"/src/bar/symdir/foo.go":  0,
			"/src/bar/symdir/symlink": os.ModeSymlink,
			"/src/foo":                os.ModeDir,
			"/src/foo/foo.go":         0,
			"/src/foo/symlink":        os.ModeSymlink,
		})
}

func TestFastWalk_Follow_SymlinkLoop(t *testing.T) {
	tempdir, err := os.MkdirTemp("", "fastwalk-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupOrLogTempDir(t, tempdir)

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
	err = fastwalk.Walk(&conf, tempdir, func(path string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if n := atomic.AddInt32(&walked, 1); n > 20 {
			return fmt.Errorf("symlink loop: %d", n)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Test that ErrTraverseLink is ignored when following symlinks
// if it would cause a symlink loop.
func TestFastWalk_Follow_ErrTraverseLink(t *testing.T) {
	conf := fastwalk.Config{
		Follow: true,
	}
	testFastWalkConf(t, &conf, map[string]string{
		"foo/foo.go": "one",
		"bar/bar.go": "two",
		"bar/symdir": "LINK:../foo/",
		"bar/loop":   "LINK:../bar/", // symlink loop
	},
		func(path string, de fs.DirEntry, err error) error {
			requireNoError(t, err)
			if err != nil {
				return err
			}
			if de.Type()&os.ModeSymlink != 0 {
				if fi, err := fastwalk.StatDirEntry(path, de); err == nil && fi.IsDir() {
					return fastwalk.ErrTraverseLink
				}
			}
			return nil
		},
		map[string]os.FileMode{
			"":                       os.ModeDir,
			"/src":                   os.ModeDir,
			"/src/bar":               os.ModeDir,
			"/src/bar/bar.go":        0,
			"/src/bar/loop":          os.ModeSymlink,
			"/src/bar/symdir":        os.ModeSymlink,
			"/src/bar/symdir/foo.go": 0,
			"/src/foo":               os.ModeDir,
			"/src/foo/foo.go":        0,
		})
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
	err := fastwalk.Walk(nil, tmp, func(_ string, _ fs.DirEntry, err error) error {
		requireNoError(t, err)
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
	err := fastwalk.Walk(nil, tmp, func(_ string, _ fs.DirEntry, err error) error {
		return err
	})
	if !os.IsNotExist(err) {
		t.Fatalf("os.IsNotExist(%+v) = false want: true", err)
	}
}

func TestFastWalk_ErrPermission(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test not supported for Windows")
	}
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
		if err := os.Chmod(dirname, 0755); err != nil {
			t.Log(err)
		}
		if err := os.Remove(dirname); err != nil {
			t.Error(err)
		}
	})

	got := map[string]os.FileMode{}
	var mu sync.Mutex
	err := fastwalk.Walk(nil, tempdir, func(path string, de fs.DirEntry, err error) error {
		if err != nil && os.IsPermission(err) {
			return nil
		}

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
		diffFileModes(t, got, want)
	}
}

func TestFastWalk_ToSlash(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("test only supported on Windows")
	}

	abs, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.ToSlash(abs)

	conf := fastwalk.Config{
		ToSlash: true,
	}
	var count atomic.Int32
	err = fastwalk.Walk(&conf, root, func(path string, de fs.DirEntry, err error) error {
		requireNoError(t, err)
		if strings.Contains(path, `\`) {
			t.Errorf("found non-forward slash separator in path: %q", path)
		}
		if _, err := de.Info(); err != nil {
			t.Fatal(err)
		}
		if _, err := de.(fastwalk.DirEntry).Stat(); err != nil {
			t.Fatal(err)
		}
		count.Add(1)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count.Load() == 0 {
		t.Fatal("did not walk any files")
	}
}

func TestFastWalk_SortMode(t *testing.T) {
	// Can only assert on files since the order that directories are
	// traversed is non-deterministic.

	tmp, err := os.MkdirTemp("", "test-fast-walk")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupOrLogTempDir(t, tmp)

	want := []string{
		"a.txt", "b.txt", "c.txt", "d.txt", "e.txt", "f.txt",
		"a.lnk", "b.lnk", "c.lnk", "d.lnk", "e.lnk", "f.lnk",
	}
	for _, name := range want {
		path := filepath.Join(tmp, name)
		if strings.HasSuffix(name, ".txt") {
			if err := writeFile(path, "data", 0666); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := symlink(t, path, path); err != nil {
				t.Fatal(err)
			}
		}
	}

	for _, mode := range []fastwalk.SortMode{
		fastwalk.SortLexical,
		fastwalk.SortFilesFirst,
		// We don't actually have any dirs because the order
		// they're visited is non-deterministic.
		fastwalk.SortDirsFirst,
	} {
		t.Run(mode.String(), func(t *testing.T) {
			want := append([]string(nil), want...)
			if mode == fastwalk.SortLexical {
				sort.Strings(want)
			}

			conf := fastwalk.Config{
				Sort: mode,
			}
			// We technically don't need a mutex since we're visiting
			// only one directory, but use it for correctness.
			var mu sync.Mutex
			var got []string
			fastwalk.Walk(&conf, tmp, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				// Ignore the parent directory
				if !d.IsDir() {
					mu.Lock()
					got = append(got, d.Name())
					mu.Unlock()
				}
				return nil
			})
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Invalid output\ngot:  %q\nwant: %q", got, want)
			}
		})
	}
}

func TestSortModeString(t *testing.T) {
	tests := []struct {
		mode fastwalk.SortMode
		want string
	}{
		{fastwalk.SortNone, "None"},
		{fastwalk.SortLexical, "Lexical"},
		{fastwalk.SortDirsFirst, "DirsFirst"},
		{fastwalk.SortFilesFirst, "FilesFirst"},
		{100, "SortMode(100)"},
		{math.MaxUint32, fmt.Sprintf("SortMode(%d)", math.MaxUint32)},
	}
	for _, test := range tests {
		got := test.mode.String()
		if got != test.want {
			t.Errorf("%d: got: %s want: %s", test.mode, got, test.want)
		}
	}
}

func TestConfigCopy(t *testing.T) {
	t.Run("Nil", func(t *testing.T) {
		c := (*fastwalk.Config)(nil).Copy()
		if c == nil {
			t.Fatal("failed to copy nil config")
		}
		if *c != (fastwalk.Config{}) {
			t.Fatal("copy of nil config should be empty")
		}
	})
	t.Run("Copy", func(t *testing.T) {
		a := fastwalk.DefaultConfig
		c := a.Copy()
		c.NumWorkers *= 2
		if a.NumWorkers == c.NumWorkers {
			t.Fatal("failed to copy config")
		}
	})
}

func TestFastWalkJoinPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("not supported on Windows")
	}
	if abs, err := filepath.Abs("/"); err != nil || abs != "/" {
		t.Skipf(`skipping filepath.Abs("/") = %q, %v; want: "/", nil`, abs, err)
	}
	sentinel := errors.New("halt now")
	var root string
	var once sync.Once
	err := fastwalk.Walk(nil, "///", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		once.Do(func() {
			root = path
		})
		return sentinel
	})
	if err != nil && err != sentinel {
		t.Fatal(err)
	}
	if root != "/" {
		t.Fatalf(`failed to convert root "///" to "/" got: %q`, root)
	}
}

func BenchmarkSortModeString(b *testing.B) {
	var s string
	for i := 0; i < b.N; i++ {
		s = fastwalk.SortMode(10).String()
	}
	if b.Failed() {
		b.Log(s)
	}
}

func diffFileModes(t *testing.T, got, want map[string]os.FileMode) {
	type Mode struct {
		Name string
		Mode os.FileMode
	}
	var extra []Mode
	for k, v := range got {
		if _, ok := want[k]; !ok {
			extra = append(extra, Mode{k, v})
		}
	}
	var missing []Mode
	for k, v := range want {
		if _, ok := got[k]; !ok {
			missing = append(missing, Mode{k, v})
		}
	}
	var delta []Mode
	for k, v := range got {
		if vv, ok := want[k]; ok && vv != v {
			delta = append(delta, Mode{k, v}, Mode{k, vv})
		}
	}
	w := new(strings.Builder)
	printMode := func(name string, modes []Mode) {
		if len(modes) == 0 {
			return
		}
		sort.Slice(modes, func(i, j int) bool {
			return modes[i].Name < modes[j].Name
		})
		if w.Len() == 0 {
			w.WriteString("\n")
		}
		fmt.Fprintf(w, "%s:\n", name)
		for _, m := range modes {
			fmt.Fprintf(w, "  %-20s: %s\n", m.Name, m.Mode.String())
		}
	}
	printMode("Extra", extra)
	printMode("Missing", missing)
	printMode("Delta", delta)
	if w.Len() != 0 {
		t.Error(w.String())
	}
}

// Directory to use for benchmarks, GOROOT is used by default
var benchDir *string

// Make sure we don't register the "benchdir" twice.
func init() {
	ff := flag.Lookup("benchdir")
	if ff != nil {
		value := ff.DefValue
		if ff.Value != nil {
			value = ff.Value.String()
		}
		benchDir = &value
	} else {
		benchDir = flag.String("benchdir", runtime.GOROOT(), "The directory to scan for BenchmarkFastWalk")
	}
}

func noopWalkFunc(_ string, _ fs.DirEntry, _ error) error { return nil }

func benchmarkFastWalk(b *testing.B, conf *fastwalk.Config,
	adapter func(fs.WalkDirFunc) fs.WalkDirFunc) {

	b.ReportAllocs()
	if adapter != nil {
		walkFn := noopWalkFunc
		for i := 0; i < b.N; i++ {
			err := fastwalk.Walk(conf, *benchDir, adapter(walkFn))
			if err != nil {
				b.Fatal(err)
			}
		}
	} else {
		for i := 0; i < b.N; i++ {
			err := fastwalk.Walk(conf, *benchDir, noopWalkFunc)
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkFastWalk(b *testing.B) {
	benchmarkFastWalk(b, nil, nil)
}

func BenchmarkFastWalkSort(b *testing.B) {
	for _, mode := range []fastwalk.SortMode{
		fastwalk.SortNone,
		fastwalk.SortLexical,
		fastwalk.SortDirsFirst,
		fastwalk.SortFilesFirst,
	} {
		b.Run(mode.String(), func(b *testing.B) {
			conf := fastwalk.DefaultConfig.Copy()
			conf.Sort = mode
			benchmarkFastWalk(b, conf, func(x fs.WalkDirFunc) fs.WalkDirFunc {
				return noopWalkFunc
			})
		})
	}
}

func BenchmarkFastWalkFollow(b *testing.B) {
	benchmarkFastWalk(b, &fastwalk.Config{Follow: true}, nil)
}

func BenchmarkFastWalkAdapters(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping: short test")
	}
	b.Run("IgnoreDuplicateDirs", func(b *testing.B) {
		benchmarkFastWalk(b, nil, fastwalk.IgnoreDuplicateDirs)
	})

	b.Run("IgnoreDuplicateFiles", func(b *testing.B) {
		benchmarkFastWalk(b, nil, fastwalk.IgnoreDuplicateFiles)
	})
}

// Benchmark various tasks with different worker counts.
//
// Observations:
//   - Linux (Intel i9-9900K / Samsung Pro NVMe): consistently benefits from
//     more workers
//   - Darwin (m1): IO heavy tasks (Readfile and Stat) and Traversal perform
//     best with 4 workers, and only CPU bound tasks benefit from more workers
func BenchmarkFastWalkNumWorkers(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping: short test")
	}

	runBench := func(b *testing.B, walkFn fs.WalkDirFunc) {
		maxWorkers := runtime.NumCPU()
		for i := 2; i <= maxWorkers; i += 2 {
			b.Run(fmt.Sprint(i), func(b *testing.B) {
				conf := fastwalk.Config{
					NumWorkers: i,
				}
				for i := 0; i < b.N; i++ {
					if err := fastwalk.Walk(&conf, *benchDir, walkFn); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}

	// Bench pure traversal speed
	b.Run("NoOp", func(b *testing.B) {
		runBench(b, func(path string, d fs.DirEntry, err error) error {
			return err
		})
	})

	// No IO and light CPU
	b.Run("NoIO", func(b *testing.B) {
		runBench(b, func(path string, d fs.DirEntry, err error) error {
			if err == nil {
				fmt.Fprintf(io.Discard, "%s: %q\n", d.Type(), path)
			}
			return err
		})
	})

	// Stat each regular file
	b.Run("Stat", func(b *testing.B) {
		runBench(b, func(path string, d fs.DirEntry, err error) error {
			if err == nil && d.Type().IsRegular() {
				_, _ = d.Info()
			}
			return err
		})
	})

	// IO heavy task
	b.Run("ReadFile", func(b *testing.B) {
		runBench(b, func(path string, d fs.DirEntry, err error) error {
			if err != nil || !d.Type().IsRegular() {
				return err
			}
			f, err := os.Open(path)
			if err != nil {
				if os.IsNotExist(err) || os.IsPermission(err) {
					return nil
				}
				return err
			}
			defer f.Close()

			_, err = io.Copy(io.Discard, f)
			return err
		})
	})

	// CPU and IO heavy task
	b.Run("Hash", func(b *testing.B) {
		bufPool := &sync.Pool{
			New: func() interface{} {
				b := make([]byte, 96*1024)
				return &b
			},
		}
		runBench(b, func(path string, d fs.DirEntry, err error) error {
			if err != nil || !d.Type().IsRegular() {
				return err
			}
			f, err := os.Open(path)
			if err != nil {
				if os.IsNotExist(err) || os.IsPermission(err) {
					return nil
				}
				return err
			}
			defer f.Close()

			p := bufPool.Get().(*[]byte)
			h := md5.New()
			_, err = io.CopyBuffer(h, f, *p)
			bufPool.Put(p)
			_ = h.Sum(nil)
			return err
		})
	})
}

var benchWalkFunc = flag.String("walkfunc", "fastwalk", "The function to use for BenchmarkWalkComparison")

// BenchmarkWalkComparison generates benchmarks using different walk functions
// so that the results can be easily compared with `benchcmp` and `benchstat`.
func BenchmarkWalkComparison(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping: short test")
	}
	switch *benchWalkFunc {
	case "fastwalk":
		benchmarkFastWalk(b, nil, nil)
	case "godirwalk":
		b.Fatal("comparisons with godirwalk are no longer supported")
	case "filepath":
		for i := 0; i < b.N; i++ {
			err := filepath.WalkDir(*benchDir, func(_ string, _ fs.DirEntry, _ error) error {
				return nil
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	default:
		b.Fatalf("invalid walkfunc: %q", *benchWalkFunc)
	}
}
