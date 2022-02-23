//go:build appengine || (!linux && !darwin && !freebsd && !openbsd && !netbsd)
// +build appengine !linux,!darwin,!freebsd,!openbsd,!netbsd

package fastwalk

import (
	"io/fs"
	"os"
)

// readDir calls fn for each directory entry in dirName.
// It does not descend into directories or follow symlinks.
// If fn returns a non-nil error, readDir returns with that error
// immediately.
func readDir(dirName string, fn func(dirName, entName string, de os.DirEntry) error) error {
	f, err := os.Open(dirName)
	if err != nil {
		return err
	}
	des, readErr := f.ReadDir(-1)
	f.Close()
	if readErr != nil && len(des) == 0 {
		return readErr
	}

	var skipFiles bool
	for _, d := range des {
		if skipFiles && d.Type().IsRegular() {
			continue
		}
		// Need to use FileMode.Type().Type() for fs.DirEntry
		e := newDirEntry(dirName, d)
		if err := fn(dirName, d.Name(), e); err != nil {
			if err != ErrSkipFiles {
				return err
			}
			skipFiles = true
		}
	}

	return readErr
}

type portableDirent struct {
	fs.DirEntry
	path string
	stat os.FileInfo
}

func (w *portableDirent) Stat() (os.FileInfo, error) {
	if w.DirEntry.Type()&os.ModeSymlink == 0 {
		return w.DirEntry.Info()
	}
	if w.stat != nil {
		return w.stat, nil
	}
	return os.Stat(w.path)
}

func statDirent(path string, de fs.DirEntry) (os.FileInfo, error) {
	if de.Type()&os.ModeSymlink == 0 {
		return de.Info()
	}
	if d, ok := de.(*portableDirent); ok && d != nil {
		fi, err := d.Stat()
		if err == nil && d.stat == nil {
			d.stat = fi
		}
		return fi, err
	}
	return os.Stat(path)
}

func newDirEntry(dirName string, info fs.DirEntry) os.DirEntry {
	return &portableDirent{
		DirEntry: info,
		path:     dirName + string(os.PathSeparator) + info.Name(),
	}
}

func fileInfoToDirEntry(dirname string, fi fs.FileInfo) fs.DirEntry {
	return newDirEntry(dirname, fs.FileInfoToDirEntry(fi))
}
