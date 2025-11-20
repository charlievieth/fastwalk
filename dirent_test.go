//go:build darwin || aix || dragonfly || freebsd || (js && wasm) || linux || netbsd || openbsd || solaris

package fastwalk_test

import (
	"io/fs"
	"os"
	"path/filepath"
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
	})
}
