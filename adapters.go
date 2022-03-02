package fastwalk

import (
	"io/fs"
	"os"
	"path/filepath"
)

func isDir(path string, d fs.DirEntry) bool {
	if d.IsDir() {
		return true
	}
	if d.Type()&os.ModeSymlink != 0 {
		if fi, err := StatDirEntry(path, d); err == nil {
			return fi.IsDir()
		}
	}
	return false
}

// TODO: the following wrappers may be incorrect since we do not
// call the WalkDirFunc in all cases.

// TODO: Rename to IgnoreDuplicateDirectories() or something
//
// WARN: remove this function since it improperly duplicates
// the logic of Config.Follow and cannot be made to pass the
// Follow tests.
//
// IgnoreDuplicateDirs wraps walkFn so that symlinks are followed.
func IgnoreDuplicateDirs(walkFn fs.WalkDirFunc) fs.WalkDirFunc {
	filter := NewEntryFilter()
	return func(path string, d fs.DirEntry, err error) error {
		werr := walkFn(path, d, err)
		if werr != nil {
			if err != filepath.SkipDir && isDir(path, d) {
				filter.Entry(path, d)
			}
			return werr
		}
		if d.Type()&os.ModeSymlink != 0 && isDir(path, d) {
			filter.Entry(path, d)
		}
		return nil

		if isDir(path, d) {
			// TODO: call walkFn on these files or is that already handled?
			if filter.Entry(path, d) {
				return filepath.SkipDir
			}
			if d.Type()&os.ModeSymlink != 0 {
				if err := walkFn(path, d, err); err != nil {
					return err
				}
				return ErrTraverseLink
			}
		}
		return walkFn(path, d, err)
		// if isDir(path, d) {
		// 	// TODO: call walkFn on these files or is that already handled?
		// 	if filter.Entry(path, d) {
		// 		return filepath.SkipDir
		// 	}
		// 	if d.Type()&os.ModeSymlink != 0 {
		// 		if err := walkFn(path, d, err); err != nil {
		// 			return err
		// 		}
		// 		return ErrTraverseLink
		// 	}
		// }
		// return walkFn(path, d, err)
	}
}

// IgnoreDuplicateFiles wraps walkFn so that symlinks are followed and duplicate
// files are ignored. If a symlink resolves to a file that has already been
// visited it will be skipped.
func IgnoreDuplicateFiles(walkFn fs.WalkDirFunc) fs.WalkDirFunc {
	filter := NewEntryFilter()
	return func(path string, d fs.DirEntry, err error) error {
		// Skip all duplicate files, directories, and links
		if filter.Entry(path, d) {
			if isDir(path, d) {
				return filepath.SkipDir
			}
			return nil
		}
		err = walkFn(path, d, err)
		// WARN: this is new
		if err == nil && d.Type()&os.ModeSymlink != 0 && isDir(path, d) {
			err = ErrTraverseLink
		}
		return err
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
