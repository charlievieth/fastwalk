//go:build appengine || (!linux && !darwin && !freebsd && !openbsd && !netbsd)
// +build appengine !linux,!darwin,!freebsd,!openbsd,!netbsd

package fastwalk

import (
	"io/fs"
	"os"
)

type portableDirent struct {
	fs.DirEntry
	path string
	stat *fileInfo
}

// TODO: cache the result of Stat
func (d *portableDirent) Stat() (os.FileInfo, error) {
	if d.DirEntry.Type()&os.ModeSymlink == 0 {
		return d.DirEntry.Info()
	}
	stat := loadFileInfo(&d.stat)
	stat.once.Do(func() {
		stat.FileInfo, stat.err = os.Stat(d.path)
	})
	return stat.FileInfo, stat.err
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
