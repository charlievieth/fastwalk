//go:build (linux || darwin || freebsd || openbsd || netbsd) && !appengine
// +build linux darwin freebsd openbsd netbsd
// +build !appengine

///////////////////////////////////////////////////////////////////////////
//
// DELETE ME DELETE ME DELETE ME DELETE ME DELETE ME DELETE ME DELETE ME
//
///////////////////////////////////////////////////////////////////////////

package fastwalk

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func requireGOROOT(t testing.TB) string {
	root := runtime.GOROOT()
	if root == "" {
		t.Fatal("GOROOT empty:", root)
	}
	fi, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if !fi.IsDir() {
		t.Fatal("GOROOT not a directory:", root)
	}
	return root
}

func BenchmarkReadDir(b *testing.B) {
	dirname := filepath.Join(requireGOROOT(b), "src")
	for i := 0; i < b.N; i++ {
		readDir(dirname, func(dirName, entName string, de DirEntry) error {
			return nil
		})
	}
}

func BenchmarkOsReadDir(b *testing.B) {
	dirname := filepath.Join(requireGOROOT(b), "src")
	for i := 0; i < b.N; i++ {
		os.ReadDir(dirname)
	}
}
