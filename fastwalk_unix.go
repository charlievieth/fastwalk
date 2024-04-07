// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build aix || dragonfly || freebsd || (js && wasm) || linux || netbsd || openbsd || solaris

package fastwalk

import (
	"io/fs"
	"os"
	"sync"
	"syscall"
	"unsafe"

	"github.com/charlievieth/fastwalk/internal/dirent"
)

// Empirical testing shows that 32k is the ideal buffer size.
const direntBufSize = 32 * 1024

var direntBufPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, direntBufSize)
		return &b
	},
}

func readDir(dirName string, fn func(dirName, entName string, de fs.DirEntry) error) error {
	fd, err := open(dirName, 0, 0)
	if err != nil {
		return &os.PathError{Op: "open", Path: dirName, Err: err}
	}
	defer syscall.Close(fd)

	pb := direntBufPool.Get().(*[]byte)
	defer direntBufPool.Put(pb)
	bbuf := *pb

	skipFiles := false
	for {
		n, err := readDirent(fd, bbuf)
		if err != nil {
			return err
		}
		if n <= 0 {
			return nil
		}
		buf := bbuf[:n:n]

		for len(buf) > 0 {
			reclen, ok := dirent.DirentReclen(buf)
			if !ok || reclen > uint64(len(buf)) {
				return nil
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
}

// According to https://golang.org/doc/go1.14#runtime
// A consequence of the implementation of preemption is that on Unix systems, including Linux and macOS
// systems, programs built with Go 1.14 will receive more signals than programs built with earlier releases.
//
// This causes syscall.Open and syscall.ReadDirent sometimes fail with EINTR errors.
// We need to retry in this case.
func open(path string, mode int, perm uint32) (fd int, err error) {
	for {
		fd, err := syscall.Open(path, mode, perm)
		if err != syscall.EINTR {
			return fd, err
		}
	}
}

func readDirent(fd int, buf []byte) (n int, err error) {
	for {
		nbuf, err := syscall.ReadDirent(fd, buf)
		if err != syscall.EINTR {
			return nbuf, err
		}
	}
}
