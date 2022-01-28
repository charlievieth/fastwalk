package fastwalk

import (
	"os"
	"sync"
	"sync/atomic"
)

const (
	stateStat = uint32(1 << iota)
	stateLstat
)

type fileInfo struct {
	info os.FileInfo
	err  error
}

// TODO: may need to move this to *_unix.go
type dirEntry struct {
	done   uint32
	mu     sync.Mutex
	typ    os.FileMode
	parent string
	name   string

	// TODO: use pointers for these fields to save space
	info *fileInfo
	stat *fileInfo
}

func (d *dirEntry) do(state uint32, fn func()) {
	if atomic.LoadUint32(&d.done)&state == 0 {
		d.doSlow(state, fn)
	}
}

func (d *dirEntry) doSlow(state uint32, fn func()) {
	d.mu.Lock()
	if done := d.done; done&state == 0 {
		fn()
		atomic.StoreUint32(&d.done, done|state)
	}
	d.mu.Unlock()
}

func (d *dirEntry) Name() string      { return d.name }
func (d *dirEntry) IsDir() bool       { return d.typ.IsDir() }
func (d *dirEntry) Type() os.FileMode { return d.typ }

func (d *dirEntry) initLstat() {
	fi, err := os.Lstat(d.parent + string(os.PathSeparator) + d.name)
	d.info = &fileInfo{fi, err}
}

func (d *dirEntry) Info() (os.FileInfo, error) {
	d.do(stateLstat, d.initLstat)
	return d.info.info, d.info.err
}

func (d *dirEntry) initStat() {
	fi, err := os.Stat(d.parent + string(os.PathSeparator) + d.name)
	d.stat = &fileInfo{fi, err}
}

func (d *dirEntry) Stat() (os.FileInfo, error) {
	// Use lstat if the entry is not a symlink
	if d.typ&os.ModeSymlink == 0 {
		return d.Info()
	}
	d.do(stateStat, d.initStat)
	return d.stat.info, d.stat.err
}

func newDirEntry(parent, name string, typ os.FileMode, lstat, stat os.FileInfo) *dirEntry {
	ude := &dirEntry{
		parent: parent,
		name:   name,
		typ:    typ,
	}
	if lstat != nil {
		ude.done |= stateLstat
		ude.info = &fileInfo{info: lstat}
	}
	if stat != nil {
		ude.done |= stateStat
		ude.stat = &fileInfo{info: stat}
	}
	return ude
}
