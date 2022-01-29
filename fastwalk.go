// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package fastwalk provides a faster version of filepath.Walk for file system
// scanning tools.
package fastwalk

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// ErrTraverseLink is used as a return value from WalkFuncs to indicate that the
// symlink named in the call may be traversed.
var ErrTraverseLink = errors.New("fastwalk: traverse symlink, assuming target is a directory")

// ErrSkipFiles is a used as a return value from WalkFuncs to indicate that the
// callback should not be called for any other files in the current directory.
// Child directories will still be traversed.
var ErrSkipFiles = errors.New("fastwalk: skip remaining files in directory")

// DefaultNumWorkers is the default number of worker goroutines to use in
// fastwalk.Walk and is in the range of 4 to 32.
var DefaultNumWorkers = func() int {
	numCPU := runtime.NumCPU()
	if numCPU < 4 {
		return 4
	}
	if numCPU > 32 {
		return 32
	}
	return numCPU
}()

// DefaultConfig is the default Config used when none is supplied.
var DefaultConfig = Config{
	Follow:     false,
	NumWorkers: DefaultNumWorkers,
}

type Config struct {
	// TODO: better document Follow

	// Follow symbolic links ignoring duplicate directories.
	Follow bool

	// Number of parallel workers to use. If NumWorkers if ≤ 0 then
	// the greater of runtime.NumCPU() or 4 is used.
	NumWorkers int
}

// A DirEntry extends the fs.DirEntry interface to add a Stat() method
// that returns the result of calling os.Stat() on the underlying file.
// The results of Info() and Stat() are cached.
//
// The fs.DirEntry argument passed to the fs.WalkDirFunc by Walk is
// always a DirEntry. The only exception is the root directory with
// with Walk is called.
type DirEntry interface {
	fs.DirEntry

	// Stat returns the FileInfo for the file or subdirectory described
	// by the entry. The returned FileInfo may be from the time of the
	// original directory read or from the time of the call to Stat.
	// If the entry denotes a symbolic link, Stat reports the information
	// about the target itself, not the link.
	Stat() (os.FileInfo, error)
}

// Walk is a faster implementation of filepath.Walk.
//
// filepath.Walk's design necessarily calls os.Lstat on each file, even if
// the caller needs less info. Many tools need only the type of each file.
// On some platforms, this information is provided directly by the readdir
// system call, avoiding the need to stat each file individually.
// fastwalk_unix.go contains a fork of the syscall routines.
//
// See golang.org/issue/16399
//
// Walk walks the file tree rooted at root, calling walkFn for each file or
// directory in the tree, including root.
//
// If walkFn returns filepath.SkipDir, the directory is skipped.
//
// Unlike filepath.Walk:
//   * File stat calls must be done by the user and should be done via
//     the DirEntry argument to walkFn since it caches the results of
//     Stat and Lstat.
//   * Multiple goroutines stat the filesystem concurrently. The provided
//     walkFn must be safe for concurrent use.
//   * Walk can follow symlinks if walkFn returns the TraverseLink
//     sentinel error. It is the walkFn's responsibility to prevent
//     Walk from going into symlink cycles.
func Walk(conf *Config, root string, walkFn fs.WalkDirFunc) error {
	if conf == nil {
		dupe := DefaultConfig
		conf = &dupe
	}
	fi, err := os.Lstat(root)
	if err != nil {
		return err
	}

	// Make sure to wait for all workers to finish, otherwise
	// walkFn could still be called after returning. This Wait call
	// runs after close(e.donec) below.
	var wg sync.WaitGroup
	defer wg.Wait()

	numWorkers := conf.NumWorkers
	if numWorkers <= 0 {
		// TODO(bradfitz): We used a minimum of 4 to give the kernel more info
		// about multiple things we want, in hopes its I/O scheduling can take
		// advantage of that. Hopefully most are in cache. Maybe 4 is even too
		// low of a minimum. Profile more.
		numWorkers = 4
		if n := runtime.NumCPU(); n > numWorkers {
			// TODO(CEV): profile to see if we still want to limit
			// concurrency on macOS.
			numWorkers = n
		}
	}

	w := &walker{
		fn:       walkFn,
		enqueuec: make(chan walkItem, numWorkers), // buffered for performance
		workc:    make(chan walkItem, numWorkers), // buffered for performance
		donec:    make(chan struct{}),

		// buffered for correctness & not leaking goroutines:
		resc: make(chan error, numWorkers),
	}
	if conf.Follow {
		w.filter = NewEntryFilter()
	}

	defer close(w.donec)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go w.doWork(&wg)
	}
	todo := []walkItem{{dir: root, info: fs.FileInfoToDirEntry(fi)}}
	out := 0
	for {
		workc := w.workc
		var workItem walkItem
		if len(todo) == 0 {
			workc = nil
		} else {
			workItem = todo[len(todo)-1]
		}
		select {
		case workc <- workItem:
			todo = todo[:len(todo)-1]
			out++
		case it := <-w.enqueuec:
			todo = append(todo, it)
		case err := <-w.resc:
			out--
			if err != nil {
				return err
			}
			if out == 0 && len(todo) == 0 {
				// It's safe to quit here, as long as the buffered
				// enqueue channel isn't also readable, which might
				// happen if the worker sends both another unit of
				// work and its result before the other select was
				// scheduled and both w.resc and w.enqueuec were
				// readable.
				select {
				case it := <-w.enqueuec:
					todo = append(todo, it)
				default:
					return nil
				}
			}
		}
	}
}

// doWork reads directories as instructed (via workc) and runs the
// user's callback function.
func (w *walker) doWork(wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-w.donec:
			return
		case it := <-w.workc:
			select {
			case <-w.donec:
				return
			case w.resc <- w.walk(it.dir, it.info, !it.callbackDone):
			}
		}
	}
}

type walker struct {
	fn fs.WalkDirFunc

	donec    chan struct{} // closed on fastWalk's return
	workc    chan walkItem // to workers
	enqueuec chan walkItem // from workers
	resc     chan error    // from workers

	filter *EntryFilter // track files when walking links
}

type walkItem struct {
	dir          string
	info         fs.DirEntry
	callbackDone bool // callback already called; don't do it again
}

func (w *walker) enqueue(it walkItem) {
	select {
	case w.enqueuec <- it:
	case <-w.donec:
	}
}

func (w *walker) onDirEnt(dirName, baseName string, de os.DirEntry) error {
	joined := dirName + string(os.PathSeparator) + baseName
	typ := de.Type()
	if typ == os.ModeSymlink && w.filter != nil {
		// Check if the symlink points to a directory before potentially
		// enqueuing it.
		if fi, _ := statDirent(joined, de); fi != nil && fi.IsDir() {
			if !w.filter.Entry(joined, de) {
				w.enqueue(walkItem{dir: joined, info: de})
			}
		}
		return nil
	}
	if typ == os.ModeDir {
		if w.filter == nil || !w.filter.Entry(joined, de) {
			w.enqueue(walkItem{dir: joined, info: de})
		}
		return nil
	}

	err := w.fn(joined, de, nil)
	if typ == os.ModeSymlink {
		// TODO: note that this only occurs when not-following symlinks
		// (aka: w.seen == nil)
		if err == ErrTraverseLink {
			// Set callbackDone so we don't call it twice for both the
			// symlink-as-symlink and the symlink-as-directory later:
			w.enqueue(walkItem{dir: joined, callbackDone: true})
			return nil
		}
		if err == filepath.SkipDir {
			// Permit SkipDir on symlinks too.
			return nil
		}
	}
	return err
}

func (w *walker) walk(root string, info fs.DirEntry, runUserCallback bool) error {
	if runUserCallback {
		err := w.fn(root, info, nil)
		if err == filepath.SkipDir {
			return nil
		}
		if err != nil {
			return err
		}
	}

	err := readDir(root, w.onDirEnt)
	if err != nil {
		// Second call, to report ReadDir error.
		return w.fn(root, info, err)
	}
	return nil
}
