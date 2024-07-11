//go:build !darwin && !(aix || dragonfly || freebsd || (js && wasm) || linux || netbsd || openbsd || solaris)

// TODO: add a "portable_dirent" build tag so that we can test this
// on non-Windows platforms

package fastwalk

import (
	"io/fs"
	"os"
	"slices"
	"strings"
	"sync"
)

var _ DirEntry = (*portableDirent)(nil)

type portableDirent struct {
	fs.DirEntry
	parent string
	stat   *fileInfo
}

func (d *portableDirent) String() string {
	return fs.FormatDirEntry(d)
}

func (d *portableDirent) Stat() (fs.FileInfo, error) {
	if d.DirEntry.Type()&os.ModeSymlink == 0 {
		return d.DirEntry.Info()
	}
	stat := loadFileInfo(&d.stat)
	stat.once.Do(func() {
		stat.FileInfo, stat.err = os.Stat(d.parent + string(os.PathSeparator) + d.Name())
	})
	return stat.FileInfo, stat.err
}

func newDirEntry(dirName string, info fs.DirEntry) DirEntry {
	return &portableDirent{
		DirEntry: info,
		parent:   dirName,
	}
}

func fileInfoToDirEntry(dirname string, fi fs.FileInfo) DirEntry {
	return newDirEntry(dirname, fs.FileInfoToDirEntry(fi))
}

var direntSlicePool = sync.Pool{
	New: func() any {
		a := make([]DirEntry, 0, 32)
		return &a
	},
}

func putDirentSlice(p *[]DirEntry) {
	// max is half as many as Unix because twice the size
	if p != nil && cap(*p) <= 16*1024 {
		a := *p
		for i := range a {
			a[i] = nil
		}
		*p = a[:0]
		direntSlicePool.Put(p)
	}
}

func sortDirents(mode SortMode, dents []DirEntry) {
	if len(dents) <= 1 {
		return
	}
	switch mode {
	case SortLexical:
		slices.SortFunc(dents, func(d1, d2 DirEntry) int {
			return strings.Compare(d1.Name(), d2.Name())
		})
	case SortFilesFirst:
		slices.SortFunc(dents, func(d1, d2 DirEntry) int {
			r1 := d1.Type().IsRegular()
			r2 := d2.Type().IsRegular()
			switch {
			case r1 && !r2:
				return -1
			case !r1 && r2:
				return 1
			case !r1 && !r2:
				// Both are not regular files: sort directories last
				dd1 := d1.Type().IsDir()
				dd2 := d2.Type().IsDir()
				switch {
				case !dd1 && dd2:
					return -1
				case dd1 && !dd2:
					return 1
				}
			}
			return strings.Compare(d1.Name(), d2.Name())
		})
	case SortDirsFirst:
		slices.SortFunc(dents, func(d1, d2 DirEntry) int {
			dd1 := d1.Type().IsDir()
			dd2 := d2.Type().IsDir()
			switch {
			case dd1 && !dd2:
				return -1
			case !dd1 && dd2:
				return 1
			case !dd1 && !dd2:
				// Both are not directories: sort regular files first
				r1 := d1.Type().IsRegular()
				r2 := d2.Type().IsRegular()
				switch {
				case r1 && !r2:
					return -1
				case !r1 && r2:
					return 1
				}
			}
			return strings.Compare(d1.Name(), d2.Name())
		})
	}
}
