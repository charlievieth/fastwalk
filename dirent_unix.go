//go:build (aix || darwin || dragonfly || freebsd || (js && wasm) || linux || netbsd || openbsd || solaris) && !appengine
// +build aix darwin dragonfly freebsd js,wasm linux netbsd openbsd solaris
// +build !appengine

package fastwalk

import (
	"io/fs"
	"os"
	"sync"
	"sync/atomic"
	"unsafe"
)

type fileInfo struct {
	once sync.Once
	os.FileInfo
	err error
}

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

func (d *unixDirent) loadFileInfo(pinfo **fileInfo) *fileInfo {
	ptr := (*unsafe.Pointer)(unsafe.Pointer(pinfo))
	fi := (*fileInfo)(atomic.LoadPointer(ptr))
	if fi == nil {
		fi = &fileInfo{}
		if !atomic.CompareAndSwapPointer(
			(*unsafe.Pointer)(unsafe.Pointer(pinfo)), // adrr
			nil,                                      // old
			unsafe.Pointer(fi),                       // new
		) {
			fi = (*fileInfo)(atomic.LoadPointer(ptr))
		}
	}
	return fi
}

func (d *unixDirent) Info() (os.FileInfo, error) {
	info := d.loadFileInfo(&d.info)
	info.once.Do(func() {
		info.FileInfo, info.err = os.Lstat(d.parent + "/" + d.name)
	})
	return info.FileInfo, info.err
}

func (d *unixDirent) Stat() (os.FileInfo, error) {
	if d.typ&os.ModeSymlink == 0 {
		return d.Info()
	}
	stat := d.loadFileInfo(&d.stat)
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

func fileInfoToDirEntry(dirname string, fi fs.FileInfo) fs.DirEntry {
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

func statDirent(path string, de fs.DirEntry) (os.FileInfo, error) {
	if de.Type()&os.ModeSymlink == 0 {
		return de.Info()
	}
	// TODO: check if the path does not match the DirEntry?
	if d, ok := de.(*unixDirent); ok {
		return d.Stat()
	}
	return os.Stat(path)
}
