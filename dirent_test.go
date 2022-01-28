//go:build (linux || darwin || freebsd || openbsd || netbsd) && !appengine
// +build linux darwin freebsd openbsd netbsd
// +build !appengine

package fastwalk

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestDirEntryOnce(t *testing.T) {
	d := new(dirEntry)
	numCPU := runtime.NumCPU()
	if numCPU < 4 {
		numCPU = 4
	}
	var nStat int32
	var nLstat int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < numCPU; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < 100; i++ {
				d.do(stateLstat, func() { atomic.AddInt32(&nLstat, 1) })
				d.do(stateStat, func() { atomic.AddInt32(&nStat, 1) })
			}
		}()
	}
	close(start)
	wg.Wait()
	if nStat != 1 {
		t.Errorf("Stat got: %d want: 1", nStat)
	}
	if nLstat != 1 {
		t.Errorf("Lstat got: %d want: 1", nLstat)
	}
}

func TestNewDirEntry(t *testing.T) {
	fi, err := os.Stat("testdata/dirent/target")
	if err != nil {
		t.Fatal(err)
	}
	d := newDirEntry("p", "n", 0, fi, nil)
	if d.done != stateLstat {
		t.Errorf("d.done want: %d got: %d", d.done, stateLstat)
	}
	d = newDirEntry("p", "n", 0, nil, fi)
	if d.done != stateStat {
		t.Errorf("d.done want: %d got: %d", d.done, stateStat)
	}
	d = newDirEntry("p", "n", 0, fi, fi)
	if d.done != stateStat|stateLstat {
		t.Errorf("d.done want: %d got: %d", d.done, stateStat|stateLstat)
	}
}

func TestDirEntry(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(wd, "testdata", "dirent")

	t.Run("Regular", func(t *testing.T) {
		path := filepath.Join(parent, "target")

		expStat, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		expLstat, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}

		de := newDirEntry(parent, "target", expLstat.Mode(), nil, nil)
		gotLstat, err := de.Info()
		if err != nil {
			t.Fatal(err)
		}
		gotStat, err := de.Stat()
		if err != nil {
			t.Fatal(err)
		}
		if err := SameFileInfo(gotLstat, expLstat); err != nil {
			t.Errorf("Lstat: %v", err)
		}
		if err := SameFileInfo(gotStat, expStat); err != nil {
			t.Errorf("Stat: %v", err)
		}
		// Stat should use Info() when the file is regular
		if gotLstat != gotStat {
			t.Errorf("Lstat != Stat:\n%#v\n%#v", gotLstat, gotStat)
		}
		if de.stat != nil {
			t.Error("Stat field set for regular file")
		}
	})

	t.Run("Symlink", func(t *testing.T) {
		path := filepath.Join(parent, "link")

		expStat, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		expLstat, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}

		de := newDirEntry(parent, "link", expLstat.Mode(), nil, nil)
		gotLstat, err := de.Info()
		if err != nil {
			t.Fatal(err)
		}
		gotStat, err := de.Stat()
		if err != nil {
			t.Fatal(err)
		}
		if err := SameFileInfo(gotLstat, expLstat); err != nil {
			t.Errorf("Lstat: %v", err)
		}
		if err := SameFileInfo(gotStat, expStat); err != nil {
			t.Errorf("Stat: %v", err)
		}
		// Stat should use Info() when the file is regular
		if gotLstat == gotStat {
			t.Errorf("Lstat == Stat:\n%#v\n%#v", gotLstat, gotStat)
		}
		if de.stat.info == nil {
			t.Error("Stat field not set for symlink")
		}
	})

	t.Run("SameFile", func(t *testing.T) {
		targetFi, err := os.Lstat(filepath.Join(parent, "target"))
		if err != nil {
			t.Fatal(err)
		}
		linkFi, err := os.Lstat(filepath.Join(parent, "link"))
		if err != nil {
			t.Fatal(err)
		}

		de1 := newDirEntry(parent, "target", targetFi.Mode(), nil, nil)
		de2 := newDirEntry(parent, "link", linkFi.Mode(), nil, nil)
		fi1, err := de1.Stat()
		if err != nil {
			t.Fatal(err)
		}
		fi2, err := de2.Stat()
		if err != nil {
			t.Fatal(err)
		}
		if v := os.SameFile(fi1, fi2); !v {
			t.Fatalf("os.SameFile(%q, %q) == %v", de1.Name(), de2.Name(), v)
		}
	})
}

func SameFileInfo(fi1, fi2 os.FileInfo) error {
	var errs []string
	if fi1.Name() != fi2.Name() {
		errs = append(errs, fmt.Sprintf("Name: %q != %q", fi1.Name(), fi2.Name()))
	}
	if fi1.Size() != fi2.Size() {
		errs = append(errs, fmt.Sprintf("Size: %d != %d", fi1.Size(), fi2.Size()))
	}
	if fi1.Mode() != fi2.Mode() {
		errs = append(errs, fmt.Sprintf("Mode: %s != %s", fi1.Mode(), fi2.Mode()))
	}
	if fi1.ModTime() != fi2.ModTime() {
		errs = append(errs, fmt.Sprintf("ModTime: %s != %s", fi1.ModTime(), fi2.ModTime()))
	}
	if fi1.IsDir() != fi2.IsDir() {
		errs = append(errs, fmt.Sprintf("IsDir: %t != %t", fi1.IsDir(), fi2.IsDir()))
	}
	if !os.SameFile(fi1, fi2) {
		errs = append(errs, "os.SameFile() == false")
	}
	var err error
	if len(errs) != 0 {
		err = fmt.Errorf("SameFileInfo(%v, %v): %s", fi1, fi2, strings.Join(errs, "; "))
	}
	return err
}

func BenchmarkDirentInfo(b *testing.B) {
	wd, err := os.Getwd()
	if err != nil {
		b.Fatal(err)
	}
	fi, err := os.Lstat("dirent_test.go")
	if err != nil {
		b.Fatal(err)
	}
	d := newDirEntry(wd, "dirent_test.go", fi.Mode(), nil, nil)
	if _, err := d.Info(); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		d.Info()
	}
}
