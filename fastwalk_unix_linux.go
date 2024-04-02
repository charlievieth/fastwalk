// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux && !appengine
// +build linux,!appengine

package fastwalk

import (
	"errors"
	"syscall"
	"unsafe"
)

func direntNamlen(dirent *syscall.Dirent) (uint64, error) {
	const fixedHdr = uint16(unsafe.Offsetof(syscall.Dirent{}.Name))
	nameBuf := (*[unsafe.Sizeof(dirent.Name)]byte)(unsafe.Pointer(&dirent.Name[0]))
	const nameBufLen = uint16(len(nameBuf))
	limit := dirent.Reclen - fixedHdr
	if limit > nameBufLen {
		limit = nameBufLen
	}
	for i := uint64(0); i < uint64(limit); i++ {
		if nameBuf[i] == 0 {
			return i, nil
		}
	}
	return 0, errors.New("failed to find terminating 0 byte in dirent")
}

func direntInode(dirent *syscall.Dirent) uint64 {
	return uint64(dirent.Ino)
}
