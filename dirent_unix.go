//go:build darwin || aix || dragonfly || freebsd || (js && wasm) || linux || netbsd || openbsd || solaris

package fastwalk

import (
	"io/fs"
	"os"
	"sort"

	"github.com/charlievieth/fastwalk/internal/fmtdirent"
)

type unixDirent struct {
	// path is the full path of the entry and is what Walk passes to walkFn.
	// name is the entry's base name and, except for the root of a walk, is a
	// substring of path rather than a separate allocation.
	path  string
	name  string
	typ   fs.FileMode
	depth uint32 // uint32 so that we can pack it next to typ
	info  *fileInfo
	stat  *fileInfo
}

func (d *unixDirent) Name() string      { return d.name }
func (d *unixDirent) IsDir() bool       { return d.typ.IsDir() }
func (d *unixDirent) Type() fs.FileMode { return d.typ }
func (d *unixDirent) Depth() int        { return int(d.depth) }
func (d *unixDirent) String() string    { return fmtdirent.FormatDirEntry(d) }

func (d *unixDirent) Info() (fs.FileInfo, error) {
	info := loadFileInfo(&d.info)
	info.once.Do(func() {
		info.FileInfo, info.err = os.Lstat(d.path)
	})
	return info.FileInfo, info.err
}

func (d *unixDirent) Stat() (fs.FileInfo, error) {
	if d.typ&os.ModeSymlink == 0 {
		return d.Info()
	}
	stat := loadFileInfo(&d.stat)
	stat.once.Do(func() {
		stat.FileInfo, stat.err = os.Stat(d.path)
	})
	return stat.FileInfo, stat.err
}

func newUnixDirent(parent, name string, typ fs.FileMode, depth int) *unixDirent {
	path := parent + "/" + name
	return &unixDirent{
		path:  path,
		name:  path[len(path)-len(name):],
		typ:   typ,
		depth: uint32(depth),
	}
}

// A direntArena holds the per-worker buffers used to build directory entries.
type direntArena struct {
	dents []*unixDirent // sort buffer, reused across directories
	ents  []unixDirent  // chunk that newDirent carves entries out of
}

// direntChunkSize is the number of unixDirents that newDirent allocates at a
// time. Entries cut from the same chunk share it, so a DirEntry the user
// retains pins its whole chunk.
const direntChunkSize = 32

// newDirent returns a directory entry for path, allocated from the worker's
// arena. name must be a substring of path.
func (wk *worker) newDirent(path, name string, typ fs.FileMode, depth int) *unixDirent {
	if len(wk.buf.ents) == 0 {
		wk.buf.ents = make([]unixDirent, direntChunkSize)
	}
	d := &wk.buf.ents[0]
	wk.buf.ents = wk.buf.ents[1:]
	*d = unixDirent{
		path:  path,
		name:  name,
		typ:   typ,
		depth: uint32(depth),
	}
	return d
}

// fileInfoToDirEntry returns a DirEntry for the file at path, which is the
// root of a walk and so may not end in fi.Name() (consider "." or "/").
func fileInfoToDirEntry(path string, fi fs.FileInfo) DirEntry {
	info := &fileInfo{
		FileInfo: fi,
	}
	info.once.Do(func() {})
	return &unixDirent{
		path: path,
		name: fi.Name(),
		typ:  fi.Mode().Type(),
		info: info,
	}
}

func sortDirents(mode SortMode, dents []*unixDirent) {
	if len(dents) <= 1 {
		return
	}
	switch mode {
	case SortLexical:
		sort.Slice(dents, func(i, j int) bool {
			return dents[i].name < dents[j].name
		})
	case SortFilesFirst:
		sort.Slice(dents, func(i, j int) bool {
			d1 := dents[i]
			d2 := dents[j]
			r1 := d1.typ.IsRegular()
			r2 := d2.typ.IsRegular()
			switch {
			case r1 && !r2:
				return true
			case !r1 && r2:
				return false
			case !r1 && !r2:
				// Both are not regular files: sort directories last
				dd1 := d1.typ.IsDir()
				dd2 := d2.typ.IsDir()
				switch {
				case !dd1 && dd2:
					return true
				case dd1 && !dd2:
					return false
				}
			}
			return d1.name < d2.name
		})
	case SortDirsFirst:
		sort.Slice(dents, func(i, j int) bool {
			d1 := dents[i]
			d2 := dents[j]
			dd1 := d1.typ.IsDir()
			dd2 := d2.typ.IsDir()
			switch {
			case dd1 && !dd2:
				return true
			case !dd1 && dd2:
				return false
			case !dd1 && !dd2:
				// Both are not directories: sort regular files first
				r1 := d1.typ.IsRegular()
				r2 := d2.typ.IsRegular()
				switch {
				case r1 && !r2:
					return true
				case !r1 && r2:
					return false
				}
			}
			return d1.name < d2.name
		})
	}
}
