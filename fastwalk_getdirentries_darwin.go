//go:build darwin && go1.13 && !nogetdirentries

package fastwalk

import (
	"io/fs"
	"os"
	"sync"
	"syscall"
	"unsafe"

	"github.com/charlievieth/fastwalk/internal/dirent"
)

// TODO: increase
const direntBufSize = 32 * 1024

var direntBufPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, direntBufSize)
		return &b
	},
}

func readDir(dirName string, fn func(dirName, entName string, de fs.DirEntry) error) error {
	fd, err := syscall.Open(dirName, syscall.O_RDONLY, 0)
	if err != nil {
		return &os.PathError{Op: "open", Path: dirName, Err: err}
	}
	defer syscall.Close(fd)

	p := direntBufPool.Get().(*[]byte)
	defer direntBufPool.Put(p)
	dbuf := *p

	var skipFiles bool
	var basep uintptr
	for {
		length, err := getdirentries(fd, dbuf, &basep)
		if err != nil {
			return &os.PathError{Op: "getdirentries64", Path: dirName, Err: err}
		}
		if length == 0 {
			break
		}
		buf := dbuf[:length]

		for i := 0; len(buf) > 0; i++ {
			reclen, ok := dirent.DirentReclen(buf)
			if !ok || reclen > uint64(len(buf)) {
				break
			}
			rec := buf[:reclen]
			buf = buf[reclen:]
			typ := dirent.DirentType(rec)
			if skipFiles && typ.IsRegular() {
				continue
			}
			const namoff = uint64(unsafe.Offsetof(syscall.Dirent{}.Name))
			namlen, ok := dirent.DirentNamlen(rec)
			if !ok || namoff+namlen > uint64(len(rec)) {
				break
			}
			name := rec[namoff : namoff+namlen]
			for i, c := range name {
				if c == 0 {
					name = name[:i]
					break
				}
			}
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
	}

	return nil
}
