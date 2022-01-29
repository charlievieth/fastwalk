//go:build darwin && go1.13 && !appengine
// +build darwin,go1.13,!appengine

package fastwalk

import (
	"flag"
	"os"
	"runtime"
	"testing"
)

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
