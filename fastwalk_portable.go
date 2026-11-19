//go:build !darwin && !(aix || dragonfly || freebsd || (js && wasm) || linux || netbsd || openbsd || solaris)

package fastwalk

import (
	"os"
	"unsafe"
)

// readDir calls fn for each directory entry in dirName.
// It does not descend into directories or follow symlinks.
// If fn returns a non-nil error, readDir returns with that error
// immediately.
func (wk *worker) readDir(dirName string, depth int) error {
	w := wk.w
	f, err := os.Open(dirName)
	if err != nil {
		return err
	}
	des, readErr := f.ReadDir(-1)
	f.Close()
	if readErr != nil && len(des) == 0 {
		return readErr
	}

	sorted := w.sortMode != SortNone
	if sorted {
		defer func() { clear(wk.dents.sortbuf); wk.dents.sortbuf = wk.dents.sortbuf[:0] }()
	}

	var skipFiles bool
	for _, d := range des {
		if skipFiles && d.Type().IsRegular() {
			continue
		}
		// Need to use FileMode.Type().Type() for fs.DirEntry
		path := wk.joinPaths(dirName, d.Name())
		e := newDirEntry(path, d, depth)
		if !sorted {
			if err := wk.onDirEnt(path, e); err != nil {
				if err != ErrSkipFiles {
					return err
				}
				skipFiles = true
			}
		} else {
			wk.dents.sortbuf = append(wk.dents.sortbuf, e)
		}
	}
	if !sorted {
		return readErr
	}

	sortDirents(w.sortMode, wk.dents.sortbuf)
	for _, d := range wk.dents.sortbuf {
		if skipFiles && d.Type().IsRegular() {
			continue
		}
		if err := wk.onDirEnt(d.(*portableDirent).path, d); err != nil {
			if err != ErrSkipFiles {
				return err
			}
			skipFiles = true
		}
	}
	return readErr
}

// joinPaths returns dir joined with base, allocated from the worker's arena.
func (wk *worker) joinPaths(dir, base string) string {
	sep := byte(os.PathSeparator)
	if os.PathSeparator != '/' && wk.w.toSlash {
		sep = '/'
	}
	// Handle the case where the root path argument to Walk is "/"
	// without this the returned path is prefixed with "//".
	if len(dir) != 0 && os.IsPathSeparator(dir[len(dir)-1]) {
		return wk.concat(dir, 0, base)
	}
	return wk.concat(dir, sep, base)
}

// concat returns dir + sep + base, allocated from the worker's arena. A zero
// sep omits the separator.
func (wk *worker) concat(dir string, sep byte, base string) string {
	n := len(dir) + len(base)
	if sep != 0 {
		n++
	}
	buf := wk.reserve(n)
	i := copy(buf, dir)
	if sep != 0 {
		buf[i] = sep
		i++
	}
	copy(buf[i:], base)
	return unsafe.String(&buf[0], n)
}
