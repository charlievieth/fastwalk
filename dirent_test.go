//go:build darwin || aix || dragonfly || freebsd || (js && wasm) || linux || netbsd || openbsd || solaris

package fastwalk_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/charlievieth/fastwalk"
)

func TestDirent(t *testing.T) {
	tempdir := t.TempDir()

	fileName := filepath.Join(tempdir, "file.txt")
	if err := writeFile(fileName, "file.txt", 0644); err != nil {
		t.Fatal(err)
	}
	linkName := filepath.Join(tempdir, "link.link")
	if err := symlink(t, filepath.Base(fileName), linkName); err != nil {
		t.Fatal(err)
	}

	// Use fastwalk.Walk to create the dir entries
	getDirEnts := func(t *testing.T) (linkEnt, fileEnt fs.DirEntry) {
		err := fastwalk.Walk(nil, tempdir, func(path string, d fs.DirEntry, err error) error {
			switch path {
			case linkName:
				linkEnt = d
			case fileName:
				fileEnt = d
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if fileEnt == nil || linkEnt == nil {
			t.Fatal("error walking directory")
		}
		return linkEnt, fileEnt
	}

	t.Run("Lstat", func(t *testing.T) {
		linkEnt, _ := getDirEnts(t)
		want, err := os.Lstat(linkName)
		if err != nil {
			t.Fatal(err)
		}
		got, err := linkEnt.Info()
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(want, got) {
			t.Errorf("lstat mismatch\n got:\n%s\nwant:\n%s",
				fastwalk.FormatFileInfo(got), fastwalk.FormatFileInfo(want))
		}
	})

	t.Run("Stat", func(t *testing.T) {
		_, fileEnt := getDirEnts(t)
		want, err := os.Stat(fileName)
		if err != nil {
			t.Fatal(err)
		}
		got, err := fastwalk.StatDirEntry(linkName, fileEnt)
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(want, got) {
			t.Errorf("lstat mismatch\n got:\n%s\nwant:\n%s",
				fastwalk.FormatFileInfo(got), fastwalk.FormatFileInfo(want))
		}
		fi, err := fileEnt.Info()
		if err != nil {
			t.Fatal(err)
		}
		if fi != got {
			t.Error("failed to return or cache FileInfo")
		}
		de := fileEnt.(fastwalk.DirEntry)
		fi, err = de.Stat()
		if err != nil {
			t.Fatal(err)
		}
		if fi != got {
			t.Error("failed to use cached Info result for non-symlink")
		}
	})

	t.Run("Parallel", func(t *testing.T) {
		testParallel := func(t *testing.T, de fs.DirEntry, fn func() (fs.FileInfo, error)) {
			numCPU := runtime.NumCPU()

			infos := make([][]fs.FileInfo, numCPU)
			for i := range infos {
				infos[i] = make([]fs.FileInfo, 100)
			}

			// Start all the goroutines at the same time to
			// maximise the chance of a race
			start := make(chan struct{})
			var wg, ready sync.WaitGroup
			ready.Add(numCPU)
			wg.Add(numCPU)
			for i := 0; i < numCPU; i++ {
				go func(fis []fs.FileInfo, de fs.DirEntry) {
					defer wg.Done()
					ready.Done()
					<-start
					for i := range fis {
						fis[i], _ = de.Info()
					}
				}(infos[i], de)
			}

			ready.Wait()
			close(start) // start all goroutines at once
			wg.Wait()

			first := infos[0][0]
			if first == nil {
				t.Fatal("failed to stat file:", de.Name())
			}
			for _, fis := range infos {
				for _, fi := range fis {
					if fi != first {
						t.Errorf("Expected the same fs.FileInfo to always "+
							"be returned got: %#v want: %#v", fi, first)
					}
				}
			}
		}

		t.Run("File", func(t *testing.T) {
			t.Run("Stat", func(t *testing.T) {
				_, fileEnt := getDirEnts(t)
				de := fileEnt.(fastwalk.DirEntry)
				testParallel(t, de, de.Stat)
			})
			t.Run("Lstat", func(t *testing.T) {
				_, fileEnt := getDirEnts(t)
				de := fileEnt.(fastwalk.DirEntry)
				testParallel(t, de, de.Info)
			})
		})

		t.Run("Link", func(t *testing.T) {
			t.Run("Stat", func(t *testing.T) {
				linkEnt, _ := getDirEnts(t)
				de := linkEnt.(fastwalk.DirEntry)
				testParallel(t, de, de.Stat)
			})
			t.Run("Lstat", func(t *testing.T) {
				linkEnt, _ := getDirEnts(t)
				de := linkEnt.(fastwalk.DirEntry)
				testParallel(t, de, de.Info)
			})
		})
	})
}
