//go:build (aix || darwin || dragonfly || freebsd || (js && wasm) || linux || netbsd || openbsd) && !appengine && !solaris
// +build aix darwin dragonfly freebsd js,wasm linux netbsd openbsd
// +build !appengine
// +build !solaris

package fastwalk

import (
	"cmp"
	"io/fs"
	"os"
	"slices"
	"sync"
)

type unixDirent struct {
	parent string
	name   string
	typ    os.FileMode
	info   *fileInfo
	stat   *fileInfo
}

func (d *unixDirent) Name() string      { return d.name }
func (d *unixDirent) IsDir() bool       { return d.typ.IsDir() }
func (d *unixDirent) Type() os.FileMode { return d.typ }

func (d *unixDirent) Info() (fs.FileInfo, error) {
	info := loadFileInfo(&d.info)
	info.once.Do(func() {
		info.FileInfo, info.err = os.Lstat(d.parent + "/" + d.name)
	})
	return info.FileInfo, info.err
}

func (d *unixDirent) Stat() (fs.FileInfo, error) {
	if d.typ&os.ModeSymlink == 0 {
		return d.Info()
	}
	stat := loadFileInfo(&d.stat)
	stat.once.Do(func() {
		stat.FileInfo, stat.err = os.Stat(d.parent + "/" + d.name)
	})
	return stat.FileInfo, stat.err
}

func newUnixDirent(parent, name string, typ os.FileMode) *unixDirent {
	return &unixDirent{
		parent: parent,
		name:   name,
		typ:    typ,
	}
}

func fileInfoToDirEntry(dirname string, fi fs.FileInfo) DirEntry {
	info := &fileInfo{
		FileInfo: fi,
	}
	info.once.Do(func() {})
	return &unixDirent{
		parent: dirname,
		name:   fi.Name(),
		typ:    fi.Mode().Type(),
		info:   info,
	}
}

type direntSlice struct {
	dirents []*unixDirent
}

var direntSlicePool = sync.Pool{
	New: func() interface{} {
		return &direntSlice{} // TODO: pre-allocate?
	},
}

func putDirentSlice(ds *direntSlice) {
	if ds != nil && len(ds.dirents) < 4096 {
		direntSlicePool.Put(ds)
	}
}

func (w *walker) readDir(dirName string) error {
	// NB: This is a bit of an odd location for this method but the logic here
	// is shared between darwin/unix and this file is included by both.
	//
	// The reason that we do not use one universal readDir method is that the
	// portable readDir does not need it (it already gets a fs.DirEntry slice).
	if !w.sort {
		return readDir(dirName, w.onDirEnt)
	}

	ds := direntSlicePool.Get().(*direntSlice)
	defer putDirentSlice(ds)
	ds.dirents = ds.dirents[:0]

	err := readDir(dirName, func(_, _ string, de DirEntry) error {
		ds.dirents = append(ds.dirents, de.(*unixDirent))
		return nil
	})
	if err != nil {
		return err // Ok to return here since the error is not from the callback
	}
	slices.SortFunc(ds.dirents, func(d1, d2 *unixDirent) int {
		return cmp.Compare(d1.name, d2.name) // TODO: is strings.Compare faster?
	})

	// TODO: switch over to use a SortMode instead to give us flexibility
	// in the future.
	//
	// NB: Processing regular files first produces a "cleaner" output at the
	// cost of not queuing more directories for parallel processing.

	// Process regular files first
	skipFiles := false
	for i, d := range ds.dirents {
		if d.typ.IsRegular() {
			ds.dirents[i] = nil // remove reference
			if skipFiles {
				continue
			}
			if err := w.onDirEnt(dirName, d.name, d); err != nil {
				if err != ErrSkipFiles {
					return err
				}
				skipFiles = true
			}
		}
	}

	// Process everything else
	for i, d := range ds.dirents {
		if d != nil {
			ds.dirents[i] = nil // remove reference
			if err := w.onDirEnt(dirName, d.name, d); err != nil {
				// WARN: ErrSkipFiles seems invalid here
				if err != ErrSkipFiles {
					return err
				}
			}
		}
	}

	return nil
}
