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
		defer func() { clear(wk.buf.dents); wk.buf.dents = wk.buf.dents[:0] }()
	}

	var skipFiles bool
	for _, d := range des {
		if skipFiles && d.Type().IsRegular() {
			continue
		}
		// Need to use FileMode.Type().Type() for fs.DirEntry
		e := newDirEntry(wk.joinPaths(dirName, d.Name()), d, depth)
		if !sorted {
			if err := wk.onDirEnt(e.(*portableDirent).path, e); err != nil {
				if err != ErrSkipFiles {
					return err
				}
				skipFiles = true
			}
		} else {
			wk.buf.dents = append(wk.buf.dents, e)
		}
	}
	if !sorted {
		return readErr
	}

	sortDirents(w.sortMode, wk.buf.dents)
	for _, d := range wk.buf.dents {
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

// joinPaths joins dir and base into a path allocated from the worker's arena.
// Paths cut from the same chunk share it, so one allocation serves many
// entries; the cost is that a path the user retains pins its whole chunk.
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
