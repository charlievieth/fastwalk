//go:build (aix || darwin || dragonfly || freebsd || (js && wasm) || linux || netbsd || openbsd || solaris) && !appengine
// +build aix darwin dragonfly freebsd js,wasm linux netbsd openbsd solaris
// +build !appengine

package fastwalk

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"unsafe"
)

func testUnixDirentParallel(t *testing.T, ent *unixDirent, want os.FileInfo,
	fn func(*unixDirent) (os.FileInfo, error)) {

	sameFile := func(fi1, fi2 os.FileInfo) bool {
		return fi1.Name() == fi2.Name() &&
			fi1.Size() == fi2.Size() &&
			fi1.Mode() == fi2.Mode() &&
			fi1.ModTime() == fi2.ModTime() &&
			fi1.IsDir() == fi2.IsDir()
		// os.SameFile(fi1, fi2)
	}

	numCPU := runtime.NumCPU()
	if numCPU < 4 {
		numCPU = 4
	}
	if numCPU > 16 {
		numCPU = 16
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	var mu sync.Mutex
	infos := make(map[*fileInfo]int)
	stats := make(map[*fileInfo]int)

	for i := 0; i < numCPU; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < 16; i++ {
				fi, err := fn(ent)
				if err != nil {
					t.Error(err)
					return
				}
				info := (*fileInfo)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&ent.info))))
				stat := (*fileInfo)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&ent.stat))))
				mu.Lock()
				infos[info]++
				stats[stat]++
				mu.Unlock()
				if !sameFile(fi, want) {
					t.Errorf("FileInfo not equal:\nwant: %s\ngot:  %s\n",
						FormatFileInfo(want), FormatFileInfo(fi))
					return
				}
			}
		}()
	}

	close(start)
	wg.Wait()

	t.Logf("Infos: %d Stats: %d\n", len(infos), len(stats))
}

func TestUnixDirent(t *testing.T) {
	tempdir := t.TempDir()

	fileName := filepath.Join(tempdir, "file.txt")
	if err := os.WriteFile(fileName, []byte("file.txt"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Run("File", func(t *testing.T) {
		fileInfo, err := os.Lstat(fileName)
		if err != nil {
			t.Fatal(err)
		}
		t.Run("Stat", func(t *testing.T) {
			ent := newUnixDirent(tempdir, filepath.Base(fileName), fileInfo.Mode().Type())
			testUnixDirentParallel(t, ent, fileInfo, (*unixDirent).Stat)
		})
		t.Run("Info", func(t *testing.T) {
			ent := newUnixDirent(tempdir, filepath.Base(fileName), fileInfo.Mode().Type())
			testUnixDirentParallel(t, ent, fileInfo, (*unixDirent).Info)
		})
	})

	t.Run("Link", func(t *testing.T) {
		linkName := filepath.Join(tempdir, "link.link")
		if err := os.Symlink(filepath.Base(fileName), linkName); err != nil {
			t.Fatal(err)
		}
		fileInfo, err := os.Lstat(linkName)
		if err != nil {
			t.Fatal(err)
		}
		t.Run("Stat", func(t *testing.T) {
			want, err := os.Stat(linkName)
			if err != nil {
				t.Fatal(err)
			}
			ent := newUnixDirent(tempdir, filepath.Base(linkName), fileInfo.Mode().Type())
			testUnixDirentParallel(t, ent, want, (*unixDirent).Stat)
		})
		t.Run("Info", func(t *testing.T) {
			ent := newUnixDirent(tempdir, filepath.Base(linkName), fileInfo.Mode().Type())
			testUnixDirentParallel(t, ent, fileInfo, (*unixDirent).Info)
		})
	})
}

func BenchmarkUnixDirentLoadFileInfo(b *testing.B) {
	wd, err := os.Getwd()
	if err != nil {
		b.Fatal(err)
	}
	fi, err := os.Lstat(wd)
	if err != nil {
		b.Fatal(err)
	}
	parent, name := filepath.Split(wd)
	d := newUnixDirent(parent, name, fi.Mode().Type())

	for i := 0; i < b.N; i++ {
		loadFileInfo(&d.info)
		d.info = nil
	}
}

func BenchmarkUnixDirentInfo(b *testing.B) {
	wd, err := os.Getwd()
	if err != nil {
		b.Fatal(err)
	}
	fi, err := os.Lstat(wd)
	if err != nil {
		b.Fatal(err)
	}
	parent, name := filepath.Split(wd)
	d := newUnixDirent(parent, name, fi.Mode().Type())

	for i := 0; i < b.N; i++ {
		fi, err := d.Info()
		if err != nil {
			b.Fatal(err)
		}
		if fi == nil {
			b.Fatal("Nil FileInfo")
		}
	}
}

func BenchmarkUnixDirentStat(b *testing.B) {
	wd, err := os.Getwd()
	if err != nil {
		b.Fatal(err)
	}
	fi, err := os.Lstat(wd)
	if err != nil {
		b.Fatal(err)
	}
	parent, name := filepath.Split(wd)
	d := newUnixDirent(parent, name, fi.Mode().Type())

	for i := 0; i < b.N; i++ {
		fi, err := d.Stat()
		if err != nil {
			b.Fatal(err)
		}
		if fi == nil {
			b.Fatal("Nil FileInfo")
		}
	}
}
