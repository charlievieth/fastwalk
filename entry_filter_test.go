package fastwalk_test

import (
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/charlievieth/fastwalk"
)

func TestEntryFilter(t *testing.T) {
	dirEntry := func(t *testing.T, name string) os.DirEntry {
		t.Helper()
		fi, err := os.Lstat(name)
		if err != nil {
			t.Fatal(err)
		}
		return fs.FileInfoToDirEntry(fi)
	}

	tempdir := t.TempDir()

	fileName := filepath.Join(tempdir, "file.txt")
	if err := writeFile(fileName, "file.txt", 0644); err != nil {
		t.Fatal(err)
	}

	linkDir := filepath.Join(tempdir, "dir")
	if err := os.Mkdir(linkDir, 0755); err != nil {
		t.Fatal(err)
	}
	linkName := filepath.Join(linkDir, "link.link")
	if err := symlink(t, "../"+filepath.Base(fileName), linkName); err != nil {
		t.Fatal(err)
	}

	// Sanity check
	{
		fi1, err := os.Stat(fileName)
		if err != nil {
			t.Fatal(err)
		}
		fi2, err := os.Stat(linkName)
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(fi1, fi2) {
			t.Fatalf("os.SameFile(%q, %q) == false", fileName, linkName)
		}
	}

	filter := fastwalk.NewEntryFilter()
	if seen := filter.Entry(fileName, dirEntry(t, fileName)); seen {
		t.Errorf("filter.Entry(%q) = %t want: %t",
			filepath.Dir(fileName), seen, false)
	}
	if seen := filter.Entry(linkName, dirEntry(t, linkName)); !seen {
		t.Errorf("filter.Entry(%q) = %t want: %t",
			filepath.Dir(linkName), seen, true)
	}

	// Entry should return true (aka seen) if there is an error
	// stat'ing the file.

	infos, err := os.ReadDir(tempdir)
	if err != nil {
		t.Fatal(err)
	}

	// Remove the files
	if err := os.RemoveAll(tempdir); err != nil {
		t.Fatal(err)
	}

	for _, de := range infos {
		// On Windows the Info field of the returned DirEntry
		// is already populated so this will succeed.
		if runtime.GOOS != "windows" {
			if _, err := de.Info(); err == nil {
				t.Fatal(de.Name())
			}
		}
		path := filepath.Join(tempdir, de.Name())
		if seen := filter.Entry(path, de); !seen {
			t.Errorf("filter.Entry(%q) = %t want: %t", de.Name(), seen, true)
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
		Info os.DirEntry
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
