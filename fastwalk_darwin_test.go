//go:build darwin && go1.13 && !appengine
// +build darwin,go1.13,!appengine

package fastwalk

import (
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"testing"
)

func TestDarwinReaddir(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadDir(wd)
	if err != nil {
		t.Fatal(err)
	}

	rdEnts, err := os.ReadDir(wd)
	if err != nil {
		t.Fatal(err)
	}

	var gdEnts []fs.DirEntry
	err = readDir(wd, func(_, _ string, de fs.DirEntry) error {
		gdEnts = append(gdEnts, de)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	sort.Slice(rdEnts, func(i, j int) bool {
		return rdEnts[i].Name() < rdEnts[j].Name()
	})
	sort.Slice(gdEnts, func(i, j int) bool {
		return gdEnts[i].Name() < gdEnts[j].Name()
	})

	sameDirEntry := func(d1, d2 fs.DirEntry) bool {
		if d1.Name() != d2.Name() || d1.IsDir() != d2.IsDir() || d1.Type() != d2.Type() {
			return false
		}
		fi1, e1 := d1.Info()
		fi2, e2 := d2.Info()
		if e1 != e2 {
			return false
		}
		return os.SameFile(fi1, fi2)
	}

	for i := range want {
		de := want[i]
		re := rdEnts[i]
		ge := gdEnts[i]
		if !sameDirEntry(de, re) {
			t.Errorf("Readir: %q: want: %#v get: %#v", de.Name(), de, re)
		}
		if !sameDirEntry(de, ge) {
			t.Errorf("Getdirentries: %q: want: %#v get: %#v", de.Name(), de, ge)
		}
	}
	if len(rdEnts) != len(want) {
		t.Errorf("Readir returned %d entries want: %d", len(rdEnts), len(want))
	}
	if len(gdEnts) != len(want) {
		t.Errorf("Getdirentries returned %d entries want: %d", len(gdEnts), len(want))
	}
}

var benchDir = flag.String("benchdir", runtime.GOROOT(), "The directory to scan for BenchmarkFastWalk")

func noopReadDirFunc(_, _ string, _ fs.DirEntry) error {
	return nil
}

func benchmarkReadDir(b *testing.B, parallel bool, fn func(dirName string, fn func(dirName, entName string, de fs.DirEntry) error) error) {
	mktemp := func(sz int) string {
		dir := filepath.Join(b.TempDir(), strconv.Itoa(sz))
		if err := os.MkdirAll(dir, 0755); err != nil {
			b.Fatal(err)
		}
		for i := 0; i < sz; i++ {
			name := strconv.Itoa(i)
			if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0644); err != nil {
				b.Fatal(err)
			}
		}
		return dir
	}
	sizes := []int{4, 8, 16, 32, 64, 128, 256}
	for _, sz := range sizes {
		dir := mktemp(sz)
		b.Run(strconv.Itoa(sz), func(b *testing.B) {
			if parallel {
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						fn(dir, noopReadDirFunc)
					}
				})
			} else {
				for i := 0; i < b.N; i++ {
					fn(dir, noopReadDirFunc)
				}
			}
		})
	}
}

func BenchmarkReadDir(b *testing.B) {
	benchmarkReadDir(b, false, readDir)
}

func BenchmarkReadDirParallel(b *testing.B) {
	dirname := *benchDir
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := readDir(dirname, noopReadDirFunc); err != nil {
				b.Fatal(err)
			}
		}
	})
}
