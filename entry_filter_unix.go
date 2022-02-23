//go:build (linux || darwin || freebsd || openbsd || netbsd || !windows) && !appengine
// +build linux darwin freebsd openbsd netbsd !windows
// +build !appengine

package fastwalk

import (
	"io/fs"
	"sync"
	"syscall"
)

type fileKey struct {
	Dev uint64
	Ino uint64
}

// TODO: use multiple maps based with dev/ino hash
// to reduce lock contention.

// An EntryFilter keeps track of visited directory entries and can be used to
// detect and avoid symlink loops or processing the same file twice.
type EntryFilter struct {
	// we assume most files have not been seen so
	// no need for a RWMutex
	mu   sync.Mutex
	keys map[fileKey]struct{}
}

// NewEntryFilter returns a new EntryFilter
func NewEntryFilter() *EntryFilter {
	return &EntryFilter{keys: make(map[fileKey]struct{}, 128)}
}

func (e *EntryFilter) seen(dev, ino uint64) (seen bool) {
	key := fileKey{
		Dev: dev,
		Ino: ino,
	}
	e.mu.Lock()
	if e.keys == nil {
		e.keys = make(map[fileKey]struct{}, 128)
	}
	_, seen = e.keys[key]
	if !seen {
		e.keys[key] = struct{}{}
	}
	e.mu.Unlock()
	return seen
}

// Entry returns if path and fs.DirEntry have been seen before.
func (e *EntryFilter) Entry(path string, de fs.DirEntry) (seen bool) {
	fi, err := statDirent(path, de)
	if err != nil {
		return true // treat errors as duplicate files
	}
	stat := fi.Sys().(*syscall.Stat_t)
	return e.seen(uint64(stat.Dev), uint64(stat.Ino))
}
