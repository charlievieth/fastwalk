//go:build !darwin && !(aix || dragonfly || freebsd || (js && wasm) || linux || netbsd || openbsd || solaris)

package fastwalk

import (
	"io/fs"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/charlievieth/fastwalk/internal/fmtdirent"
)

var _ DirEntry = dirEntry{}

// Minimal DirEntry for testing
type dirEntry struct {
	name string
	typ  fs.FileMode
}

func (de dirEntry) Name() string               { return de.name }
func (de dirEntry) IsDir() bool                { return de.typ.IsDir() }
func (de dirEntry) Type() fs.FileMode          { return de.typ.Type() }
func (de dirEntry) Info() (fs.FileInfo, error) { panic("not implemented") }
func (de dirEntry) Stat() (fs.FileInfo, error) { panic("not implemented") }

func (de dirEntry) String() string {
	return fmtdirent.FormatDirEntry(de)
}

// NB: this must be kept in sync with the
// TestSortDirents in dirent_unix_test.go
func TestSortDirents(t *testing.T) {
	direntNames := func(dents []DirEntry) []string {
		names := make([]string, len(dents))
		for i, d := range dents {
			names[i] = d.Name()
		}
		return names
	}

	t.Run("None", func(t *testing.T) {
		dents := []DirEntry{
			dirEntry{name: "b"},
			dirEntry{name: "a"},
			dirEntry{name: "d"},
			dirEntry{name: "c"},
		}
		want := direntNames(dents)
		sortDirents(SortNone, dents)
		got := direntNames(dents)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got: %q want: %q", got, want)
		}
	})

	rr := rand.New(rand.NewSource(time.Now().UnixNano()))
	shuffleDirents := func(dents []DirEntry) []DirEntry {
		rr.Shuffle(len(dents), func(i, j int) {
			dents[i], dents[j] = dents[j], dents[i]
		})
		return dents
	}

	// dents needs to be in the expected order
	test := func(t *testing.T, dents []DirEntry, mode SortMode) {
		want := direntNames(dents)
		// Run multiple times with different shuffles
		for i := 0; i < 10; i++ {
			t.Run("", func(t *testing.T) {
				sortDirents(mode, shuffleDirents(dents))
				got := direntNames(dents)
				if !reflect.DeepEqual(got, want) {
					t.Errorf("got: %q want: %q", got, want)
				}
			})
		}
	}

	t.Run("Lexical", func(t *testing.T) {
		dents := []DirEntry{
			dirEntry{name: "a"},
			dirEntry{name: "b"},
			dirEntry{name: "c"},
			dirEntry{name: "d"},
		}
		test(t, dents, SortLexical)
	})

	t.Run("FilesFirst", func(t *testing.T) {
		dents := []DirEntry{
			// Files lexically
			dirEntry{name: "f1", typ: 0},
			dirEntry{name: "f2", typ: 0},
			dirEntry{name: "f3", typ: 0},
			// Non-dirs lexically
			dirEntry{name: "a1", typ: fs.ModeSymlink},
			dirEntry{name: "a2", typ: fs.ModeSymlink},
			dirEntry{name: "a3", typ: fs.ModeSymlink},
			dirEntry{name: "s1", typ: fs.ModeSocket},
			dirEntry{name: "s2", typ: fs.ModeSocket},
			dirEntry{name: "s3", typ: fs.ModeSocket},
			// Dirs lexically
			dirEntry{name: "d1", typ: fs.ModeDir},
			dirEntry{name: "d2", typ: fs.ModeDir},
			dirEntry{name: "d3", typ: fs.ModeDir},
		}
		test(t, dents, SortFilesFirst)
	})

	t.Run("DirsFirst", func(t *testing.T) {
		dents := []DirEntry{
			// Dirs lexically
			dirEntry{name: "d1", typ: fs.ModeDir},
			dirEntry{name: "d2", typ: fs.ModeDir},
			dirEntry{name: "d3", typ: fs.ModeDir},
			// Files lexically
			dirEntry{name: "f1", typ: 0},
			dirEntry{name: "f2", typ: 0},
			dirEntry{name: "f3", typ: 0},
			// Non-dirs lexically
			dirEntry{name: "a1", typ: fs.ModeSymlink},
			dirEntry{name: "a2", typ: fs.ModeSymlink},
			dirEntry{name: "a3", typ: fs.ModeSymlink},
			dirEntry{name: "s1", typ: fs.ModeSocket},
			dirEntry{name: "s2", typ: fs.ModeSocket},
			dirEntry{name: "s3", typ: fs.ModeSocket},
		}
		test(t, dents, SortDirsFirst)
	})
}
