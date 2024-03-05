//go:build appengine || solaris || (!linux && !darwin && !freebsd && !openbsd && !netbsd)
// +build appengine solaris !linux,!darwin,!freebsd,!openbsd,!netbsd

package fastwalk

import (
	"cmp"
	"io/fs"
	"os"
	"slices"
)

// readDir calls fn for each directory entry in dirName.
// It does not descend into directories or follow symlinks.
// If fn returns a non-nil error, readDir returns with that error
// immediately.
func (w *walker) readDir(dirName string) error {
	f, err := os.Open(dirName)
	if err != nil {
		return err
	}
	des, readErr := f.ReadDir(-1)
	f.Close()
	if readErr != nil && len(des) == 0 {
		return readErr
	}

	if w.sort {
		slices.SortFunc(des, func(d1, d2 fs.DirEntry) int {
			return cmp.Compare(d1.Name(), d2.Name())
		})
	}

	var skipFiles bool
	for _, d := range des {
		if skipFiles && d.Type().IsRegular() {
			continue
		}
		// Need to use FileMode.Type().Type() for fs.DirEntry
		e := newDirEntry(dirName, d)
		if err := w.onDirEnt(dirName, d.Name(), e); err != nil {
			if err != ErrSkipFiles {
				return err
			}
			skipFiles = true
		}
	}

	return readErr
}
