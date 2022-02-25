//go:build darwin && go1.13 && !appengine
// +build darwin,go1.13,!appengine

package fastwalk

import (
	"flag"
	"os"
	"runtime"
	"sort"
	"testing"
)

func TestDarwinReaddir(t *testing.T) {
	if useGetdirentries == false {
		t.Skip("Skipping due to 'nogetdirentries' build tag")
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadDir(wd)
	if err != nil {
		t.Fatal(err)
	}

	var rdEnts []os.DirEntry
	err = readDir_Readir(wd, func(_, _ string, de os.DirEntry) error {
		rdEnts = append(rdEnts, de)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var gdEnts []os.DirEntry
	err = readDir_Getdirentries(wd, func(_, _ string, de os.DirEntry) error {
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

	sameDirEntry := func(d1, d2 os.DirEntry) bool {
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

func noopReadDirFunc(_, _ string, _ os.DirEntry) error {
	return nil
}

func BenchmarkReadDir_Getdirentries(b *testing.B) {
	dirname := *benchDir
	for i := 0; i < b.N; i++ {
		if err := readDir_Getdirentries(dirname, noopReadDirFunc); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadDir_Getdirentries_Parallel(b *testing.B) {
	dirname := *benchDir
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := readDir_Getdirentries(dirname, noopReadDirFunc); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkReadDir_Readir(b *testing.B) {
	dirname := *benchDir
	for i := 0; i < b.N; i++ {
		if err := readDir_Readir(dirname, noopReadDirFunc); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadDir_Readdir_Parallel(b *testing.B) {
	dirname := *benchDir
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := readDir_Readir(dirname, noopReadDirFunc); err != nil {
				b.Fatal(err)
			}
		}
	})
}
