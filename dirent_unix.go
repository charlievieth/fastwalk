//go:build (aix || darwin || dragonfly || freebsd || (js && wasm) || linux || netbsd || openbsd || solaris) && !appengine
// +build aix darwin dragonfly freebsd js,wasm linux netbsd openbsd solaris
// +build !appengine

package fastwalk

import (
	"io/fs"
	"os"
)

type fileInfo struct {
	os.FileInfo
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

func (d *unixDirent) Info() (os.FileInfo, error) {
	if d.info != nil {
		return d.info.FileInfo, nil
	}
	return os.Lstat(d.parent + "/" + d.name)
}

func (d *unixDirent) Stat() (os.FileInfo, error) {
	if d.typ&os.ModeSymlink == 0 {
		return d.Info()
	}
	if d.stat != nil {
		return d.stat.FileInfo, nil
	}
	return os.Stat(d.parent + "/" + d.name)
}

func newUnixDirent(parent, name string, typ os.FileMode) *unixDirent {
	return &unixDirent{
		parent: parent,
		name:   name,
		typ:    typ,
	}
}

func lstatDirent(path string, de fs.DirEntry) (os.FileInfo, error) {
	// TODO: check if the path does not match the DirEntry?
	if d, ok := de.(*unixDirent); ok && d != nil {
		fi, err := d.Info()
		if err == nil && d.info == nil {
			d.info = &fileInfo{fi}
		}
		return fi, err
	}
	return de.Info()
}

func statDirent(path string, de fs.DirEntry) (os.FileInfo, error) {
	if de.Type()&os.ModeSymlink == 0 {
		return lstatDirent(path, de)
	}
	// TODO: check if the path does not match the DirEntry?
	if d, ok := de.(*unixDirent); ok && d != nil {
		fi, err := d.Stat()
		if err == nil && d.stat == nil {
			d.stat = &fileInfo{fi}
		}
		return fi, err
	}
	return os.Stat(path)
}
