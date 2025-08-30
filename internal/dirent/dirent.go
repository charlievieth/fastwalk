//go:build aix || dragonfly || freebsd || (js && wasm) || linux || netbsd || openbsd || solaris

package dirent

import (
	"encoding/binary"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

const InvalidMode = os.FileMode(1<<32 - 1)

// readInt returns the size-bytes unsigned integer in native byte order at offset off.
func readInt(b []byte, off, size uintptr) (uint64, bool) {
	if len(b) < int(off+size) {
		return 0, false
	}
	b = b[off:]
	switch size {
	case 1:
		return uint64(b[0]), true
	case 2:
		return uint64(binary.NativeEndian.Uint16(b)), true
	case 4:
		return uint64(binary.NativeEndian.Uint32(b)), true
	case 8:
		return uint64(binary.NativeEndian.Uint64(b)), true
	default:
		// This case is impossible if the tests pass. Previously, we panicked
		// here but for performance reasons (escape analysis) it's better to
		// trigger a panic via an out-of-bounds reference.
		_ = b[len(b)]
		return 0, false
	}
}

func Parse(buf []byte) (consumed int, name string, typ os.FileMode) {

	reclen, ok := direntReclen(buf)
	if !ok || reclen > uint64(len(buf)) {
		// WARN: this is a hard error because we consumed 0 bytes
		// and not stopping here could lead to an infinite loop.
		return 0, "", InvalidMode
	}
	consumed = int(reclen)
	rec := buf[:reclen]

	ino, ok := direntIno(rec)
	if !ok {
		return consumed, "", InvalidMode
	}
	// When building to wasip1, the host runtime might be running on Windows
	// or might expose a remote file system which does not have the concept
	// of inodes. Therefore, we cannot make the assumption that it is safe
	// to skip entries with zero inodes.
	if ino == 0 && runtime.GOOS != "wasip1" {
		return consumed, "", InvalidMode
	}

	typ = direntType(buf)

	const namoff = uint64(unsafe.Offsetof(syscall.Dirent{}.Name))
	namlen, ok := direntNamlen(rec)
	if !ok || namoff+namlen > uint64(len(rec)) {
		return consumed, "", InvalidMode
	}
	namebuf := rec[namoff : namoff+namlen]
	for i, c := range namebuf {
		if c == 0 {
			namebuf = namebuf[:i]
			break
		}
	}
	// Check for useless names before allocating a string.
	if string(namebuf) == "." {
		name = "."
	} else if string(namebuf) == ".." {
		name = ".."
	} else {
		name = string(namebuf)
	}
	return consumed, name, typ
}
