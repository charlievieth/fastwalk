package fastwalk

import (
	"io/fs"
	"os"
	"syscall"
)

// StatDirEntry returns a [fs.FileInfo] describing the named file. If de is a
// [fastwalk.DirEntry] its Stat method is used. Otherwise, [os.Stat] is called
// on the path. Therefore, de should be the DirEntry describing path.
//
// Note: This function was added when fastwalk used to always cache the result
// of DirEntry.Stat. Now that fastwalk no longer explicitly caches the result
// of Stat this function is slightly less useful and is equivalent to the below
// code:
//
//	if d, ok := de.(DirEntry); ok {
//		return d.Stat()
//	}
//	return os.Stat(path)
func StatDirEntry(path string, de fs.DirEntry) (fs.FileInfo, error) {
	if de == nil {
		return nil, &os.PathError{Op: "stat", Path: path, Err: syscall.EINVAL}
	}
	if d, ok := de.(DirEntry); ok {
		return d.Stat()
	}
	return os.Stat(path)
}

// DirEntryDepth returns the depth at which entry de was generated relative
// to the root being walked or -1 if de does not have type [fastwalk.DirEntry].
//
// This is a helper function that saves the user from having to cast the
// [fs.DirEntry] argument to their walk function to a [fastwalk.DirEntry]
// and is equivalent to the below code:
//
//	if d, _ := de.(DirEntry); d != nil {
//		return d.Depth()
//	}
//	return -1
func DirEntryDepth(de fs.DirEntry) int {
	if d, _ := de.(DirEntry); d != nil {
		return d.Depth()
	}
	return -1
}
