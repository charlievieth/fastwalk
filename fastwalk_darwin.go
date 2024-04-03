//go:build darwin && go1.13 && !appengine
// +build darwin,go1.13,!appengine

package fastwalk

import (
	"io/fs"
	"os"
	"sync"
	"syscall"
	"unsafe"
)

//sys	closedir(dir uintptr) (err error)
//sys	readdir_r(dir uintptr, entry *Dirent, result **Dirent) (res Errno)

func readDir(dirName string, fn func(dirName, entName string, de fs.DirEntry) error) error {
	fd, err := opendir(dirName)
	if err != nil {
		return &os.PathError{Op: "opendir", Path: dirName, Err: err}
	}
	defer closedir(fd) //nolint:errcheck

	skipFiles := false
	var dirent syscall.Dirent
	var entptr *syscall.Dirent
	for {
		if errno := readdir_r(fd, &dirent, &entptr); errno != 0 {
			if errno == syscall.EINTR {
				continue
			}
			return &os.PathError{Op: "readdir", Path: dirName, Err: errno}
		}
		if entptr == nil { // EOF
			break
		}
		if dirent.Ino == 0 {
			continue
		}
		typ := dtToType(dirent.Type)
		if skipFiles && typ.IsRegular() {
			continue
		}
		name := (*[len(syscall.Dirent{}.Name)]byte)(unsafe.Pointer(&dirent.Name))[:]
		for i, c := range name {
			if c == 0 {
				name = name[:i]
				break
			}
		}
		// Check for useless names before allocating a string.
		if string(name) == "." || string(name) == ".." {
			continue
		}
		nm := string(name)
		if err := fn(dirName, nm, newUnixDirent(dirName, nm, typ)); err != nil {
			if err != ErrSkipFiles {
				return err
			}
			skipFiles = true
		}
	}

	return nil
}

const direntBufSize = 32 * 1024

var direntBufPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, direntBufSize)
		return &b
	},
}

func dtToType(typ uint8) os.FileMode {
	switch typ {
	case syscall.DT_BLK:
		return os.ModeDevice
	case syscall.DT_CHR:
		return os.ModeDevice | os.ModeCharDevice
	case syscall.DT_DIR:
		return os.ModeDir
	case syscall.DT_FIFO:
		return os.ModeNamedPipe
	case syscall.DT_LNK:
		return os.ModeSymlink
	case syscall.DT_REG:
		return 0
	case syscall.DT_SOCK:
		return os.ModeSocket
	}
	return ^os.FileMode(0)
}

// Copied from syscall/syscall_unix.go

// Do the interface allocations only once for common
// Errno values.
var (
	errEAGAIN error = syscall.EAGAIN
	errEINVAL error = syscall.EINVAL
	errENOENT error = syscall.ENOENT
)

// errnoErr returns common boxed Errno values, to prevent
// allocations at runtime.
func errnoErr(e syscall.Errno) error {
	switch e {
	case 0:
		return nil
	case syscall.EAGAIN:
		return errEAGAIN
	case syscall.EINVAL:
		return errEINVAL
	case syscall.ENOENT:
		return errENOENT
	}
	return e
}
