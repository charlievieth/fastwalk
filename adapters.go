package fastwalk

import (
	"io/fs"
	"os"
	"path/filepath"
)

func isDir(path string, d fs.DirEntry) bool {
	typ := d.Type()
	if typ&os.ModeSymlink != 0 {
		if fi, err := StatDirEntry(path, d); err == nil {
			typ = fi.Mode().Type()
		}
	}
	return typ.IsDir()
}

// TODO: the following wrappers may be incorrect since we do not
// call the WalkDirFunc in all cases.

// FollowSymlinks wraps walkFn so that symlinks are followed.
func FollowSymlinks(walkFn fs.WalkDirFunc) fs.WalkDirFunc {
	filter := NewEntryFilter()
	return func(path string, d fs.DirEntry, err error) error {
		if isDir(path, d) {
			// TODO: call walkFn on these files or is that already handled?
			if filter.Entry(path, d) {
				return filepath.SkipDir
			}
			if d.Type()&os.ModeSymlink != 0 {
				return ErrTraverseLink
			}
			return nil
		}
		return walkFn(path, d, err)
	}
}

// IgnoreDuplicateFiles wraps walkFn so that symlinks are followed and duplicate
// files are ignored. If a symlink resolves to a file that has already been
// visited it will be skipped.
func IgnoreDuplicateFiles(walkFn fs.WalkDirFunc) fs.WalkDirFunc {
	filter := NewEntryFilter()
	return func(path string, ent fs.DirEntry, err error) error {
		typ := ent.Type()
		if filter.Entry(path, ent) {
			if typ.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if typ&os.ModeSymlink != 0 {
			return ErrTraverseLink
		}
		return walkFn(path, ent, err)
	}
}

// IgnorePermissionErrors wraps walkFn so that permission errors are ignored.
func IgnorePermissionErrors(walkFn fs.WalkDirFunc) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {
		if err != nil && os.IsPermission(err) {
			return nil
		}
		return walkFn(path, d, err)
	}
}
