//go:build (linux && !appengine) || darwin || freebsd || openbsd || netbsd || !windows
// +build linux,!appengine darwin freebsd openbsd netbsd !windows

package fastwalk

import (
	"sync"
	"syscall"
)

type fileKey struct {
	Dev uint64
	Ino uint64
}

// TODO: use multiple maps based with dev/ino hash
// to reduce lock contention.

type EntryFilter struct {
	// we assume most files have not been seen so
	// no need for a RWMutex
	mu   sync.Mutex
	keys map[fileKey]struct{}
}

func NewEntryFilter() *EntryFilter {
	return &EntryFilter{keys: make(map[fileKey]struct{}, 128)}
}

func (e *EntryFilter) seen(dev, ino uint64) bool {
	key := fileKey{
		Dev: dev,
		Ino: ino,
	}
	e.mu.Lock()
	if e.keys == nil {
		e.keys = make(map[fileKey]struct{}, 128)
	}
	_, ok := e.keys[key]
	if !ok {
		e.keys[key] = struct{}{}
	}
	e.mu.Unlock()
	return ok
}

func (e *EntryFilter) Entry(_ string, de DirEntry) bool {
	fi, err := de.Stat()
	if err != nil {
		return true // treat errors as duplicate files
	}
	stat := fi.Sys().(*syscall.Stat_t)
	return e.seen(uint64(stat.Dev), uint64(stat.Ino))
}

// type xseen struct {
// 	devs atomic.Value // map[uint64]*InoMap
// 	mu   sync.Mutex
// 	p    map[uint64]*InoMap
// 	dev  sync.Map
// }

// func (x *xseen) load() map[uint64]*InoMap {
// 	m := *(*map[uint64]*InoMap)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&x.p))))
// 	if m == nil {
// 		x.mu.Lock()
// 		if x.p == nil {
// 			m = make(map[uint64]*InoMap)
// 			dst := (*unsafe.Pointer)(unsafe.Pointer(&x.p))
// 			val := unsafe.Pointer(m)
// 			atomic.StorePointer(dst, val)
// 			// atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&x.p), unsafe.Pointer(m)))
// 		}
// 		x.mu.Unlock()
// 	}
// 	return m
// }

// func (x *xseen) init() map[uint64]*InoMap {
// 	// TODO: can we use CompareAndSwap()
// 	x.mu.Lock()
// 	devs, _ := x.devs.Load().(map[uint64]*InoMap)
// 	if devs == nil {
// 		devs = make(map[uint64]*InoMap)
// 		x.devs.Store(devs)
// 	}
// 	x.mu.Unlock()
// 	return devs
// }

// func (x *xseen) addIno(ino uint64) map[uint64]struct{} {
// 	x.mu.Lock()
// 	m := x.devs[ino]
// 	return nil
// }

// type DevMap struct {
// 	firstDev uint64
// 	first    *InoMap
// 	small    [8]*InoMap
// 	size     uint32 // WARN
// 	m        map[uint64]*InoMap
// }

// func (d *DevMap) Load(dev uint64) *InoMap {
// 	return nil
// }

// type InoMap struct {
// 	mu   sync.Mutex
// 	inos map[uint64]struct{}
// }

// func (m *InoMap) Seen(ino uint64) (seen bool) {
// 	if m != nil {
// 		m.mu.Lock()
// 		_, seen = m.inos[ino]
// 		if !seen {
// 			m.inos[ino] = struct{}{}
// 		}
// 		m.mu.Unlock()
// 	}
// 	return seen
// }

// func (x *xseen) seen(dev, ino uint64) bool {
// 	// xx := x.devs.Load().(map[uint64]*InoMap)[dev]
// 	// _ = xx
//
// 	// TODO: don't lazily initialize
// 	devs, _ := x.devs.Load().(map[uint64]*InoMap)
// 	if devs == nil {
// 		devs = x.init()
// 	}
// 	inos := devs[dev]
// 	if inos == nil {
// 		x.mu.Lock()
// 		// WARN: need to recheck the condition!!!
// 		inos = &InoMap{inos: make(map[uint64]struct{})}
// 		m := make(map[uint64]*InoMap, len(devs)+1)
// 		for k, v := range devs {
// 			m[k] = v
// 		}
// 		m[ino] = inos
// 		devs = m
// 		x.devs.Store(devs)
// 		x.mu.Unlock()
// 	}
// 	return false
// }

// func (s *EntryFilter) Seen(path string) bool {
// 	var stat syscall.Stat_t
// 	var err error
// 	for {
// 		err = syscall.Stat(path, &stat)
// 		if err != syscall.EINTR {
// 			break
// 		}
// 	}
// 	if err != nil {
// 		return false
// 	}
// 	return s.seen(uint64(stat.Dev), uint64(stat.Ino))
// }

// func (s *EntryFilter) Dir(path string) bool {
// 	var stat syscall.Stat_t
// 	var err error
// 	for {
// 		err = syscall.Stat(path, &stat)
// 		if err != syscall.EINTR {
// 			break
// 		}
// 	}
// 	if err != nil {
// 		return false
// 	}
// 	if stat.Mode&syscall.S_IFMT != syscall.S_IFDIR {
// 		return false
// 	}
// 	return s.seen(uint64(stat.Dev), uint64(stat.Ino))
// }
