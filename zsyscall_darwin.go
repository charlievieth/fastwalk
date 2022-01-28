// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build darwin && go1.12
// +build darwin,go1.12

package fastwalk

import (
	"syscall"
	"unsafe"
	_ "unsafe"
)

// TODO(charlie): implement the closedir and readdir_r functions here instead
// of linking into the stdlib? This is what the golang.org/x/sys/unix package
// does.

// Implemented in syscall/syscall_darwin.go.

//go:linkname closedir syscall.closedir
func closedir(dir uintptr) (err error)

//go:linkname readdir_r syscall.readdir_r
func readdir_r(dir uintptr, entry *syscall.Dirent, result **syscall.Dirent) (res syscall.Errno)

// Implent opendir so that we don't have to open a file, duplicate it's FD,
// then call fdopendir with it.

func opendir(path string) (dir uintptr, err error) {
	p, err := syscall.BytePtrFromString(path)
	if err != nil {
		return 0, err
	}
	r0, _, e1 := syscall_syscallPtr(libc_opendir_trampoline_addr, uintptr(unsafe.Pointer(p)), 0, 0)
	dir = uintptr(r0)
	if e1 != 0 {
		err = errnoErr(e1)
	}
	return dir, err
}

var libc_opendir_trampoline_addr uintptr

//go:cgo_import_dynamic libc_opendir opendir "/usr/lib/libSystem.B.dylib"

// Implemented in the runtime package (runtime/sys_darwin.go)
func syscall_syscallPtr(fn, a1, a2, a3 uintptr) (r1, r2 uintptr, err syscall.Errno)

//go:linkname syscall_syscallPtr syscall.syscallPtr
