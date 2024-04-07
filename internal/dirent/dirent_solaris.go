//go:build solaris

package dirent

import (
	"os"
	"syscall"
	"unsafe"
)

func DirentIno(buf []byte) (uint64, bool) {
	return readInt(buf, unsafe.Offsetof(syscall.Dirent{}.Ino), unsafe.Sizeof(syscall.Dirent{}.Ino))
}

func DirentReclen(buf []byte) (uint64, bool) {
	return readInt(buf, unsafe.Offsetof(syscall.Dirent{}.Reclen), unsafe.Sizeof(syscall.Dirent{}.Reclen))
}

func DirentNamlen(buf []byte) (uint64, bool) {
	reclen, ok := DirentReclen(buf)
	if !ok {
		return 0, false
	}
	return reclen - uint64(unsafe.Offsetof(syscall.Dirent{}.Name)), true
}

func DirentType(buf []byte) os.FileMode {
	return ^os.FileMode(0) // unknown
}
