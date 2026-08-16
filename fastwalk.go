// Package fastwalk provides a faster version of [filepath.WalkDir] for file
// system scanning tools.
package fastwalk

/*
 * This code borrows heavily from golang.org/x/tools/internal/fastwalk
 * and as such the Go license can be found in the go.LICENSE file and
 * is reproduced below:
 *
 * Copyright (c) 2009 The Go Authors. All rights reserved.
 *
 * Redistribution and use in source and binary forms, with or without
 * modification, are permitted provided that the following conditions are
 * met:
 *
 *    * Redistributions of source code must retain the above copyright
 * notice, this list of conditions and the following disclaimer.
 *    * Redistributions in binary form must reproduce the above
 * copyright notice, this list of conditions and the following disclaimer
 * in the documentation and/or other materials provided with the
 * distribution.
 *    * Neither the name of Google Inc. nor the names of its
 * contributors may be used to endorse or promote products derived from
 * this software without specific prior written permission.
 *
 * THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
 * "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
 * LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
 * A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
 * OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
 * SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
 * LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
 * DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
 * THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
 * (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
 * OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
 */

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
)

// ErrTraverseLink is used as a return value from WalkDirFuncs to indicate that
// the symlink named in the call may be traversed. This error is ignored if
// the Follow [Config] option is true.
var ErrTraverseLink = errors.New("fastwalk: traverse symlink, assuming target is a directory")

// ErrSkipFiles is a used as a return value from WalkFuncs to indicate that the
// callback should not be called for any other files in the current directory.
// Child directories will still be traversed.
var ErrSkipFiles = errors.New("fastwalk: skip remaining files in directory")

// SkipDir is used as a return value from WalkDirFuncs to indicate that
// the directory named in the call is to be skipped. It is not returned
// as an error by any function.
var SkipDir = fs.SkipDir

// TODO(charlie): Look into implementing the fs.SkipAll behavior of
// filepath.Walk and filepath.WalkDir. This may not be possible without taking
// a performance hit.

// DefaultNumWorkers returns the default number of worker goroutines to use in
// [Walk] and is the value of [runtime.GOMAXPROCS](-1) clamped to a range
// of 4 to 32 except on Darwin where it is either 4 (8 cores or less), 6
// (10 cores or less), or 10 (more than 10 cores). This is because Walk / IO
// performance on Darwin degrades with more concurrency.
//
// The optimal number for your workload may be lower or higher. The results
// of BenchmarkFastWalkNumWorkers benchmark may be informative.
func DefaultNumWorkers() int {
	numCPU := runtime.GOMAXPROCS(-1)
	if numCPU < 4 {
		return 4
	}
	// User manually set GOMAXPROCS - respect it.
	if numCPU != runtime.NumCPU() {
		return min(numCPU, 32)
	}
	// Darwin IO performance on APFS can slow with increased parallelism.
	// For Intel CPUs (and maybe older arm64 CPUs) performance is best
	// around 4-10 workers and file IO is best around 4 workers. More workers
	// only benefit CPU intensive tasks.
	//
	// As of macOS 15 (on ARM Macs), the parallel performance of readdir_r(3)
	// and stat(2) calls has improved and the ideal number of workers is now
	// generally the number of performance cores.
	if runtime.GOOS == "darwin" {
		if n := darwinNumPerfCores(); n > 0 {
			return n
		}
		// This is primarily for Intel CPUs.
		switch {
		case numCPU <= 8:
			return 4
		case numCPU <= 10:
			return 6
		default: // numCPU > 10
			return 10
		}
	}
	return min(numCPU, 32)
}

// DefaultToSlash returns true if this is a Go program compiled for Windows
// running in an environment ([MSYS/MSYS2] or [Git for Windows]) that uses
// forward slashes as the path separator instead of the native backslash.
//
// On non-Windows OSes this is a no-op and always returns false.
//
// To detect if we're running in [MSYS/MSYS2] we check if the "MSYSTEM"
// environment variable exists.
//
// DefaultToSlash does not detect if this is a Windows executable running in [WSL].
// Instead, users should (ideally) use programs compiled for Linux in WSL.
//
// See: [github.com/junegunn/fzf/issues/3859]
//
// NOTE: The reason that we do not check if we're running in WSL is that the
// test was inconsistent since it depended on the working directory (it seems
// that "/proc" cannot be accessed when programs are ran from a mounted Windows
// directory) and what environment variables are shared between WSL and Win32
// (this requires explicit [configuration]).
//
// [MSYS/MSYS2]: https://www.msys2.org/
// [WSL]: https://learn.microsoft.com/en-us/windows/wsl/about
// [Git for Windows]: https://gitforwindows.org/
// [github.com/junegunn/fzf/issues/3859]: https://github.com/junegunn/fzf/issues/3859
// [configuration]: https://devblogs.microsoft.com/commandline/share-environment-vars-between-wsl-and-windows/
func DefaultToSlash() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	// Previously this function attempted to determine if this is a Windows exe
	// running in WSL. The check was:
	//
	// * File /proc/sys/fs/binfmt_misc/WSLInterop exist
	// * Env var "WSL_DISTRO_NAME" exits
	// * /proc/version contains "Microsoft" or "microsoft"
	//
	// Below are my notes explaining why that check was flaky:
	//
	// NOTE: This appears to fail when ran from WSL when the current working
	// directory is a Windows directory that is mounted ("/mnt/c/...") since
	// "/proc" is not accessible. It works if ran from a directory that is not
	// mounted. Additionally, the "WSL_DISTRO_NAME" environment variable is not
	// set when ran from WSL.
	//
	// I'm not sure what causes this, but it would be great to find a solution.
	// My guess is that when ran from a Windows directory it uses the native
	// Windows path syscalls (for example os.Getwd reports the canonical Windows
	// path when a Go exe is ran from a mounted directory in WSL, but reports the
	// WSL path when ran from outside a mounted Windows directory).
	//
	// That said, the real solution here is to use programs compiled for Linux
	// when running in WSL.
	_, ok := os.LookupEnv("MSYSTEM")
	return ok
}

// SortMode determines the order that a directory's entries are visited by
// [Walk]. Sorting applies only at the directory level and since we process
// directories in parallel the order in which all files are visited is still
// non-deterministic.
//
// Sorting is mostly useful for programs that print the output of Walk since
// it makes it slightly more ordered compared to the default directory order.
// Sorting may also help some programs that wish to change the order in which
// a directory is processed by either processing all files first or enqueuing
// all directories before processing files.
//
// All lexical sorting is case-sensitive.
//
// The overhead of sorting is minimal compared to the syscalls needed to
// walk directories. The impact on performance due to changing the order
// in which directory entries are processed will be dependent on the workload
// and the structure of the file tree being visited (it might also have no
// impact).
type SortMode uint32

const (
	// Perform no sorting. Files will be visited in directory order.
	// This is the default.
	SortNone SortMode = iota

	// Directory entries are sorted by name before being visited.
	SortLexical

	// Sort the directory entries so that regular files and non-directories
	// (e.g. symbolic links) are visited before directories. Within each
	// group (regular files, other files, directories) the entries are sorted
	// by name.
	//
	// This is likely the mode that programs that print the output of Walk
	// want to use. Since by processing all files before enqueuing
	// sub-directories the output is slightly more grouped.
	//
	// Example order:
	//   - file: "a.txt"
	//   - file: "b.txt"
	//   - link: "a.link"
	//   - link: "b.link"
	//   - dir:  "d1/"
	//   - dir:  "d2/"
	//
	SortFilesFirst

	// Sort the directory entries so that directories are visited first, then
	// regular files are visited, and finally everything else is visited
	// (e.g. symbolic links). Within each group (directories, regular files,
	// other files) the entries are sorted by name.
	//
	// This mode is might be useful at preventing other walk goroutines from
	// stalling due to lack of work since it immediately enqueues all of a
	// directory's sub-directories for processing. The impact on performance
	// will be dependent on the workload and the structure of the file tree
	// being visited - it might also have no (or even a negative) impact on
	// performance so testing/benchmarking is recommend.
	//
	// An example workload that might cause this is: processing one directory
	// takes a long time, that directory has sub-directories we want to walk,
	// while processing that directory all other Walk goroutines have finished
	// processing their directories, those goroutines are now stalled waiting
	// for more work (waiting on the one running goroutine to enqueue its
	// sub-directories for processing).
	//
	// This might also be beneficial if processing files is expensive.
	//
	// Example order:
	//   - dir:  "d1/"
	//   - dir:  "d2/"
	//   - file: "a.txt"
	//   - file: "b.txt"
	//   - link: "a.link"
	//   - link: "b.link"
	//
	SortDirsFirst
)

var sortModeStrs = [...]string{
	SortNone:       "None",
	SortLexical:    "Lexical",
	SortDirsFirst:  "DirsFirst",
	SortFilesFirst: "FilesFirst",
}

func (s SortMode) String() string {
	if 0 <= int(s) && int(s) < len(sortModeStrs) {
		return sortModeStrs[s]
	}
	return "SortMode(" + itoa(uint64(s)) + ")"
}

// DefaultConfig is the default [Config] used when none is supplied.
var DefaultConfig = Config{
	Follow:     false,
	ToSlash:    DefaultToSlash(),
	NumWorkers: DefaultNumWorkers(),
	Sort:       SortNone,
	MaxDepth:   0,
}

// A Config controls the behavior of [Walk].
type Config struct {
	// TODO: do we want to pass a sentinel error to WalkFunc if
	// a symlink loop is detected?

	// Follow symbolic links ignoring directories that would lead
	// to infinite loops; that is, entering a previously visited
	// directory that is an ancestor of the last file encountered.
	//
	// The sentinel error ErrTraverseLink is ignored when Follow
	// is true (this to prevent users from defeating the loop
	// detection logic), but SkipDir and ErrSkipFiles are still
	// respected.
	Follow bool

	// Join all paths using a forward slash "/" instead of the system
	// default (the root path will be converted with filepath.ToSlash).
	// This option exists for users on Windows Subsystem for Linux (WSL)
	// that are running a Windows executable (like FZF) in WSL and need
	// forward slashes for compatibility (since binary was compiled for
	// Windows the path separator will be "\" which can cause issues in
	// in a Unix shell).
	//
	// This option has no effect when the OS path separator is a
	// forward slash "/".
	//
	// See FZF issue: https://github.com/junegunn/fzf/issues/3859
	ToSlash bool

	// Sort a directory's entries by SortMode before visiting them.
	// The order that files are visited is deterministic only at the directory
	// level, but not generally deterministic because we process directories
	// in parallel. The performance impact of sorting entries is generally
	// negligible compared to the syscalls required to read directories.
	//
	// This option mostly exists for programs that print the output of Walk
	// (like FZF) since it provides some order and thus makes the output much
	// nicer compared to the default directory order, which is basically random.
	Sort SortMode

	// Number of parallel workers to use. If NumWorkers if ≤ 0 then
	// DefaultNumWorkers is used.
	NumWorkers int

	// MaxDepth limits the depth of directory traversal to MaxDepth levels
	// beyond the root directory being walked. By default, there is no limit
	// on the search depth and a value of zero or less disables this feature.
	MaxDepth int
}

// Copy returns a copy of c. If c is nil an empty [Config] is returned.
func (c *Config) Copy() *Config {
	dupe := new(Config)
	if c != nil {
		*dupe = *c
	}
	return dupe
}

// A DirEntry extends the [fs.DirEntry] interface to add a Stat() method
// that returns the result of calling [os.Stat] on the underlying file.
// The results of Info() and Stat() are cached.
//
// The [fs.DirEntry] argument passed to the [fs.WalkDirFunc] by [Walk] is
// always a DirEntry.
type DirEntry interface {
	fs.DirEntry

	// Stat returns the fs.FileInfo for the file or subdirectory described
	// by the entry. The returned FileInfo may be from the time of the
	// original directory read or from the time of the call to os.Stat.
	// If the entry denotes a symbolic link, Stat reports the information
	// about the target itself, not the link.
	Stat() (fs.FileInfo, error)

	// Depth returns the depth at which this entry was generated relative to the
	// root being walked.
	Depth() int
}

// Walk is a faster implementation of [filepath.WalkDir] that walks the file
// tree rooted at root in parallel, calling walkFn for each file or directory
// in the tree, including root.
//
// All errors that arise visiting files and directories are filtered by walkFn
// see the [fs.WalkDirFunc] documentation for details.
// The [IgnorePermissionErrors] adapter is provided to handle to common case of
// ignoring [fs.ErrPermission] errors.
//
// By default files are walked in directory order, which makes the output
// non-deterministic. The Sort [Config] option can be used to control the order
// in which directory entries are visited, but since we walk the file tree in
// parallel the output is still non-deterministic (it's just slightly more
// sorted).
//
// When a symbolic link is encountered, by default Walk will not follow it
// unless walkFn returns [ErrTraverseLink] or the Follow [Config] setting is
// true. See below for a more detailed explanation.
//
// Walk calls walkFn with paths that use the separator character appropriate
// for the operating system unless the ToSlash [Config] setting is true which
// will cause all paths to be joined with a forward slash.
//
// If walkFn returns the [SkipDir] sentinel error, the directory is skipped.
// If walkFn returns the [ErrSkipFiles] sentinel error, the callback will not
// be called for any other files in the current directory. Unlike,
// [filepath.Walk] and [filepath.WalkDir] the [fs.SkipAll] sentinel error is
// not respected.
//
// Unlike [filepath.WalkDir]:
//
//   - Multiple goroutines stat the filesystem concurrently. The provided
//     walkFn must be safe for concurrent use.
//
//   - The order that directories are visited is non-deterministic.
//
//   - File stat calls must be done by the user and should be done via
//     the [DirEntry] argument to walkFn. The [DirEntry] caches the result
//     of both Info() and Stat(). The Stat() method is a fastwalk specific
//     extension and can be called by casting the [fs.DirEntry] to a
//     [fastwalk.DirEntry] or via the [StatDirEntry] helper. The [fs.DirEntry]
//     argument to walkFn will always be convertible to a [fastwalk.DirEntry].
//
//   - The [fs.DirEntry] argument is always a [fastwalk.DirEntry], which has
//     a Stat() method that returns the result of calling [os.Stat] on the
//     file. The result of Stat() and Info() are cached. The [StatDirEntry]
//     helper can be used to call Stat() on the returned [fastwalk.DirEntry].
//
//   - Additionally, the [fs.DirEntry] argument (which has type [fastwalk.DirEntry]),
//     has a Depth() method that returns the depth at which the entry was generated
//     relative to the root being walked. The [DirEntryDepth] helper function
//     can be used to call Depth() on the [fs.DirEntry] argument.
//
//   - Walk can follow symlinks in two ways: the fist, and simplest, is to
//     set Follow [Config] option to true - this will cause Walk to follow
//     symlinks and detect/ignore any symlink loops; the second, is for walkFn
//     to return the sentinel [ErrTraverseLink] error.
//     When using [ErrTraverseLink] to follow symlinks it is walkFn's
//     responsibility to prevent Walk from going into symlink cycles.
//     By default Walk does not follow symbolic links.
//
//   - When walking a directory, walkFn will be called for each non-directory
//     entry and directories will be enqueued and visited at a later time or
//     by another goroutine.
//
//   - The [fs.SkipAll] sentinel error is not respected and not ignored. If the
//     WalkDirFunc returns SkipAll then Walk will exit with the error SkipAll.
//
//   - walkFn runs on the calling goroutine as well as on the workers Walk
//     starts, so a panic from walkFn may surface on either.
//
//   - Paths and [DirEntry] values are allocated in blocks rather than one at a
//     time. Retaining a path or a DirEntry that walkFn was passed therefore
//     keeps a small fixed amount of memory (on the order of a kilobyte) alive
//     alongside it. Callers that keep a small subset of a large walk and care
//     about the difference should copy what they keep, e.g. with
//     [strings.Clone].
func Walk(conf *Config, root string, walkFn fs.WalkDirFunc) error {
	fi, err := os.Stat(root)
	if err != nil {
		return err
	}
	if conf == nil {
		dupe := DefaultConfig
		conf = &dupe
	}
	if conf.ToSlash {
		root = filepath.ToSlash(root)
	}

	numWorkers := conf.NumWorkers
	if numWorkers <= 0 {
		numWorkers = DefaultNumWorkers()
	}

	w := &walker{
		fn: walkFn,
		// TODO: we should just pass the Config
		maxDepth:   conf.MaxDepth,
		numWorkers: numWorkers,
		follow:     conf.Follow,
		toSlash:    conf.ToSlash,
		sortMode:   conf.Sort,
	}
	w.cond.L = &w.mu
	if w.follow {
		w.ignoredDirs = append(w.ignoredDirs, fi)
	}

	// Make sure to wait for all workers to finish, otherwise walkFn could
	// still be called after returning. stop() is idempotent and is called
	// here so that the workers exit if walkFn panics.
	defer func() {
		w.stop(nil)
		w.wg.Wait()
	}()

	root = cleanRootPath(root)

	// The calling goroutine runs as the first worker; the rest are started
	// on demand as work becomes available (see (*worker).publish). Walking a
	// small tree never pays for goroutines it cannot use.
	wk := worker{w: w}
	w.pending.Store(1)
	w.started = 1
	wk.local = append(wk.local, walkItem{
		dir:  root,
		info: fileInfoToDirEntry(root, fi),
	})
	wk.run()

	// All work is accounted for, but other workers may still be inside
	// walkFn. Wait for them before reporting the result.
	w.wg.Wait()
	w.mu.Lock()
	err = w.err
	w.mu.Unlock()
	return err
}

// A walker holds the state shared by all of a Walk's workers.
//
// Work is distributed with a shared LIFO stack guarded by mu. A worker keeps
// one newly discovered directory to itself and publishes the rest, so it can
// descend one level without synchronizing at all while everything else stays
// available to whichever worker frees up first. Workers block on cond when
// they run out of work and are woken by a publish or by the end of the walk.
type walker struct {
	fn fs.WalkDirFunc

	// pending counts the walkItems that have been created but not yet
	// processed, wherever they currently live (shared stack, a worker's
	// private stack, or in flight). The walk is complete when it hits zero.
	pending atomic.Int64

	mu      sync.Mutex
	cond    sync.Cond
	stack   []walkItem // shared work stack (LIFO)
	idle    int        // workers blocked in cond.Wait
	started int        // running workers
	done    bool
	err     error
	wg      sync.WaitGroup

	ignoredDirs []fs.FileInfo
	maxDepth    int
	numWorkers  int
	follow      bool
	toSlash     bool
	sortMode    SortMode
}

// stop ends the walk, recording err as the result if it is the first error
// seen. It is safe to call multiple times.
func (w *walker) stop(err error) {
	w.mu.Lock()
	if w.err == nil {
		w.err = err
	}
	if !w.done {
		w.done = true
		w.cond.Broadcast()
	}
	w.mu.Unlock()
}

type walkItem struct {
	dir          string
	info         DirEntry
	callbackDone bool // callback already called; don't do it again
}

// A worker processes directories. Its private state (work stack, sort buffer,
// and the arenas used to allocate paths and directory entries) is owned by a
// single goroutine and needs no synchronization.
type worker struct {
	w     *walker
	local []walkItem // private work stack (LIFO)

	// Directory entries are allocated from arenas rather than one at a time.
	// This trades a bounded amount of retained memory (a DirEntry that the
	// user holds on to pins its whole chunk) for far fewer allocations.
	buf   direntArena // sort buffer and entry chunk (platform specific)
	arena []byte      // chunk that path strings are carved out of
}

func (wk *worker) run() {
	w := wk.w
	for {
		it, ok := wk.next()
		if !ok {
			return
		}
		if err := wk.walk(it.dir, it.info, !it.callbackDone); err != nil {
			w.stop(err)
			return
		}
		// Hand off work before retiring this item so that pending cannot
		// reach zero while another worker still has something to do.
		wk.publish()
		if w.pending.Add(-1) == 0 {
			w.stop(nil)
			return
		}
	}
}

// next returns the next directory to process, blocking until one is available.
// It reports false once the walk has finished or been stopped.
func (wk *worker) next() (walkItem, bool) {
	if n := len(wk.local); n > 0 {
		it := wk.local[n-1]
		wk.local[n-1] = walkItem{}
		wk.local = wk.local[:n-1]
		return it, true
	}
	w := wk.w
	w.mu.Lock()
	for {
		if w.done {
			w.mu.Unlock()
			return walkItem{}, false
		}
		if n := len(w.stack); n > 0 {
			it := w.stack[n-1]
			w.stack[n-1] = walkItem{}
			w.stack = w.stack[:n-1]
			w.mu.Unlock()
			return it, true
		}
		w.idle++
		w.cond.Wait()
		w.idle--
	}
}

// enqueue adds a directory to this worker's private stack.
func (wk *worker) enqueue(it walkItem) {
	// Count the child before its parent is retired so that pending never
	// dips to zero while the walk is still live.
	wk.w.pending.Add(1)
	wk.local = append(wk.local, it)
}

// publish hands all but one of this worker's pending directories to the shared
// stack and starts new workers up to the configured limit.
//
// Holding a single item back is what makes the private stack worth having: the
// worker descends into it without going through the lock at all. Holding more
// than one is measurably worse. It turns the walk into an independent
// depth-first search per worker, which spreads the workers across distant
// branches of the tree; opendir(3) then costs ~12% more, presumably because
// path resolution no longer benefits from other workers having just walked the
// same parent directories.
func (wk *worker) publish() {
	n := len(wk.local)
	if n < 2 {
		return
	}
	k := n - 1 // hand off everything but the deepest entry
	w := wk.w
	w.mu.Lock()
	if w.done {
		w.mu.Unlock()
		return
	}
	w.stack = append(w.stack, wk.local[:k]...)
	// Idle workers take the first k items; start new ones for the rest.
	for i := w.idle; i < k && w.started < w.numWorkers; i++ {
		w.started++
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			wk := worker{w: w}
			wk.run()
		}()
	}
	for i := 0; i < k && i < w.idle; i++ {
		w.cond.Signal()
	}
	w.mu.Unlock()

	wk.local[0] = wk.local[k]
	clear(wk.local[1:n]) // drop the references left in the tail
	wk.local = wk.local[:1]
}

func (w *walker) shouldSkipDir(fi fs.FileInfo) bool {
	for _, ignored := range w.ignoredDirs {
		if os.SameFile(ignored, fi) {
			return true
		}
	}
	return false
}

func (w *walker) shouldTraverse(path string, de DirEntry) bool {
	ts, err := de.Stat()
	if err != nil {
		return false
	}
	if !ts.IsDir() {
		return false
	}
	if w.shouldSkipDir(ts) {
		return false
	}
	for {
		parent := filepath.Dir(path)
		if parent == path {
			return true
		}
		parentInfo, err := os.Stat(parent)
		if err != nil {
			return false
		}
		if os.SameFile(ts, parentInfo) {
			return false
		}
		path = parent
	}
}

// arenaChunkSize is the size of the byte slices that paths are carved
// strings out of; paths longer than this get a dedicated allocation. It is
// deliberately small: a path the caller retains keeps its whole chunk alive,
// and measurement shows nothing to gain from larger chunks.
const arenaChunkSize = 1024

// joinPathBytes is joinPaths for a base name that is still in a raw directory
// buffer. It returns the full path along with the base name as a substring of
// it, so neither costs an allocation of its own.
func (wk *worker) joinPathBytes(dir string, base []byte) (path, name string) {
	sep := byte(os.PathSeparator)
	if os.PathSeparator != '/' && wk.w.toSlash {
		sep = '/'
	}
	if len(dir) != 0 && os.IsPathSeparator(dir[len(dir)-1]) {
		sep = 0
	}
	n := len(dir) + len(base)
	if sep != 0 {
		n++
	}
	buf := wk.reserve(n)
	i := copy(buf, dir)
	if sep != 0 {
		buf[i] = sep
		i++
	}
	copy(buf[i:], base)
	path = unsafe.String(&buf[0], n)
	return path, path[len(path)-len(base):]
}

// reserve returns an n byte slice carved out of the worker's arena. The
// returned bytes are never handed out again, so it is safe to build a string
// on top of them.
func (wk *worker) reserve(n int) []byte {
	if n > len(wk.arena) {
		size := arenaChunkSize
		if n > size {
			size = n
		}
		wk.arena = make([]byte, size)
	}
	buf := wk.arena[:n:n]
	wk.arena = wk.arena[n:]
	return buf
}

func (wk *worker) onDirEnt(joined string, de DirEntry) error {
	w := wk.w
	typ := de.Type()
	if typ == os.ModeDir {
		wk.enqueue(walkItem{dir: joined, info: de})
		return nil
	}

	err := w.fn(joined, de, nil)
	if typ == os.ModeSymlink {
		if err == ErrTraverseLink {
			if !w.follow {
				// Set callbackDone so we don't call it twice for both the
				// symlink-as-symlink and the symlink-as-directory later:
				wk.enqueue(walkItem{dir: joined, info: de, callbackDone: true})
				return nil
			}
			err = nil // Ignore ErrTraverseLink when Follow is true.
		}
		if err == filepath.SkipDir {
			// Permit SkipDir on symlinks too.
			return nil
		}
		if err == nil && w.follow && w.shouldTraverse(joined, de) {
			// Traverse symlink
			wk.enqueue(walkItem{dir: joined, info: de, callbackDone: true})
		}
	}
	return err
}

func (wk *worker) walk(root string, info DirEntry, runUserCallback bool) error {
	w := wk.w
	if runUserCallback {
		err := w.fn(root, info, nil)
		if err == filepath.SkipDir {
			return nil
		}
		if err != nil {
			return err
		}
	}

	depth := info.Depth()
	if w.maxDepth > 0 && depth >= w.maxDepth {
		return nil
	}
	err := wk.readDir(root, depth+1)
	if err != nil {
		// Second call, to report ReadDir error.
		return w.fn(root, info, err)
	}
	return nil
}

// cleanRootPath returns the root path trimmed of extraneous trailing slashes.
// This is a no-op on Windows.
func cleanRootPath(root string) string {
	if runtime.GOOS == "windows" || len(filepath.VolumeName(root)) != 0 {
		// Windows paths or any path with a volume name (which AFAIK should
		// only be Windows) are a bit too complicated to clean.
		return root
	}
	if len(filepath.VolumeName(root)) != 0 {
		return root
	}
	for i := len(root) - 1; i >= 0; i-- {
		if !os.IsPathSeparator(root[i]) {
			return root[:i+1]
		}
	}
	if root != "" {
		return root[0:1] // root is all path separators ("//")
	}
	return root
}

// Avoid the dependency on strconv since it pulls in a large number of other
// dependencies which bloats the size of this package.
func itoa(val uint64) string {
	buf := make([]byte, 20)
	i := len(buf) - 1
	for val >= 10 {
		buf[i] = byte(val%10 + '0')
		i--
		val /= 10
	}
	buf[i] = byte(val + '0')
	return string(buf[i:])
}
