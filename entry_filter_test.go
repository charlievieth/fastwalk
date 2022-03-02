package fastwalk_test

import (
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/charlievieth/fastwalk"
)

func TestEntryFilter(t *testing.T) {
	tempdir := t.TempDir()
	files := map[string]string{
		"foo/foo.go": "one",
		"bar/bar.go": "LINK:../foo/foo.go",
		"bar/baz.go": "two",
		"bar/loop":   "LINK:../bar/", // symlink loop
		"file.go":    "three",

		// Use multiple symdirs to increase the chance that one
		// of these and not "foo" is followed first.
		"symdir1": "LINK:foo",
		"symdir2": "LINK:foo",
		"symdir3": "LINK:foo",
		"symdir4": "LINK:foo",
	}
	testCreateFiles(t, tempdir, files)

	var mu sync.Mutex
	var seen []os.FileInfo
	filter := fastwalk.NewEntryFilter()
	walkFn := fastwalk.IgnoreDuplicateFiles(func(path string, de fs.DirEntry, err error) error {
		requireNoError(t, err)
		fi1, err := fastwalk.StatDirEntry(path, de)
		if err != nil {
			t.Error(err)
			return err
		}
		mu.Lock()
		defer mu.Unlock()
		if !filter.Entry(path, de) {
			for _, fi2 := range seen {
				if os.SameFile(fi1, fi2) {
					t.Errorf("Visited file twice: %q (%s) and %q (%s)",
						path, fi1.Mode(), fi2.Name(), fi2.Mode())
				}
			}
		}
		seen = append(seen, fi1)
		return nil
	})
	if err := fastwalk.Walk(nil, tempdir, walkFn); err != nil {
		t.Fatal(err)
	}

	// Test that true is returned for a non-existent directory
	// On Windows the Info field of the returned DirEntry
	// is already populated so this will succeed.
	if runtime.GOOS != "windows" {
		path := filepath.Join(tempdir, "src", "foo/foo.go")
		fi, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if !filter.Entry(path, fs.FileInfoToDirEntry(fi)) {
			t.Error("EntryFilter should return true when the file does not exist")
		}
	}
}

func BenchmarkEntryFilter(b *testing.B) {
	tempdir := b.TempDir()

	names := make([]string, 0, 2048)
	for i := 0; i < 1024; i++ {
		name := filepath.Join(tempdir, fmt.Sprintf("dir_%04d", i))
		if err := os.Mkdir(name, 0755); err != nil {
			b.Fatal(err)
		}
		names = append(names, name)
	}
	for i := 0; i < 1024; i++ {
		name := filepath.Join(tempdir, fmt.Sprintf("file_%04d", i))
		if err := writeFile(name, filepath.Base(name), 0644); err != nil {
			b.Fatal(err)
		}
		names = append(names, name)
	}
	rr := rand.New(rand.NewSource(time.Now().UnixNano()))
	rr.Shuffle(len(names), func(i, j int) {
		names[i], names[j] = names[j], names[i]
	})

	type fileInfo struct {
		Name string
		Info fs.DirEntry
	}
	infos := make([]fileInfo, len(names))
	for i, name := range names {
		fi, err := os.Lstat(name)
		if err != nil {
			b.Fatal(err)
		}
		infos[i] = fileInfo{name, fs.FileInfoToDirEntry(fi)}
	}

	b.ResetTimer()

	b.Run("MostlyHits", func(b *testing.B) {
		filter := fastwalk.NewEntryFilter()
		for i := 0; i < b.N; i++ {
			x := infos[i%len(infos)]
			filter.Entry(x.Name, x.Info)
		}
	})

	b.Run("MostlyHitsParallel", func(b *testing.B) {
		filter := fastwalk.NewEntryFilter()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				x := infos[i%len(infos)]
				filter.Entry(x.Name, x.Info)
				i++
			}
		})
	})

	b.Run("HalfMisses", func(b *testing.B) {
		filter := fastwalk.NewEntryFilter()
		n := len(infos)
		for i := 0; i < b.N; i++ {
			x := infos[i%len(infos)]
			filter.Entry(x.Name, x.Info)
			if i != 0 && i%n == 0 {
				b.StopTimer()
				filter = fastwalk.NewEntryFilter()
				b.StartTimer()
			}
		}
	})
}
