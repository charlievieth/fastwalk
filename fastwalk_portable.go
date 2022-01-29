// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

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

func lstatDirent(_ string, d fs.DirEntry) (os.FileInfo, error) {
	return d.Info() // this is no-op on Windows
}

func statDirent(path string, d fs.DirEntry) (os.FileInfo, error) {
	if d.Type()&os.ModeSymlink == 0 {
		return lstatDirent(path, d)
	}
	if w, ok := d.(*portableDirent); ok && w != nil {
		fi, err := w.Stat()
		if err == nil && w.stat == nil {
			w.stat = fi
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
