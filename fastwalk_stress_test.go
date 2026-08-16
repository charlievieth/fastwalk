package fastwalk_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/charlievieth/fastwalk"
)

// buildTree makes a tree with an awkward shape: a long fanout-1 chain (which
// starves the work queue), a very wide directory, and a deep balanced part.
func buildTree(t testing.TB) (root string, want []string) {
	root = t.TempDir()
	mk := func(p string) {
		if err := os.MkdirAll(p, 0755); err != nil {
			t.Fatal(err)
		}
	}
	touch := func(p string) {
		if err := os.WriteFile(p, nil, 0644); err != nil {
			t.Fatal(err)
		}
	}
	// fanout-1 chain
	chain := filepath.Join(root, "chain")
	for i := 0; i < 40; i++ {
		chain = filepath.Join(chain, fmt.Sprintf("c%d", i))
		mk(chain)
		touch(filepath.Join(chain, "f"))
	}
	// wide directory
	wide := filepath.Join(root, "wide")
	mk(wide)
	for i := 0; i < 200; i++ {
		mk(filepath.Join(wide, fmt.Sprintf("d%03d", i)))
		touch(filepath.Join(wide, fmt.Sprintf("f%03d", i)))
	}
	// balanced tree
	var rec func(dir string, depth int)
	rec = func(dir string, depth int) {
		if depth == 0 {
			return
		}
		for i := 0; i < 3; i++ {
			d := filepath.Join(dir, fmt.Sprintf("b%d", i))
			mk(d)
			touch(filepath.Join(d, "leaf"))
			rec(d, depth-1)
		}
	}
	mk(filepath.Join(root, "bal"))
	rec(filepath.Join(root, "bal"), 4)
	// empty dirs
	for i := 0; i < 10; i++ {
		mk(filepath.Join(root, fmt.Sprintf("empty%d", i)))
	}

	err := filepath.WalkDir(root, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		want = append(want, p)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(want)
	return root, want
}

// The walk must visit every entry exactly once, at every worker count, and
// must always terminate.
func TestWalkStressCoverage(t *testing.T) {
	root, want := buildTree(t)
	for _, workers := range []int{1, 2, 3, 4, 8, 16, 64} {
		t.Run(fmt.Sprint(workers), func(t *testing.T) {
			for iter := 0; iter < 20; iter++ {
				var mu sync.Mutex
				got := make([]string, 0, len(want))
				conf := &fastwalk.Config{NumWorkers: workers}
				err := fastwalk.Walk(conf, root, func(p string, _ fs.DirEntry, err error) error {
					if err != nil {
						return err
					}
					mu.Lock()
					got = append(got, p)
					mu.Unlock()
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
				sort.Strings(got)
				if len(got) != len(want) {
					t.Fatalf("iter %d: visited %d entries; want %d", iter, len(got), len(want))
				}
				for i := range got {
					if got[i] != want[i] {
						t.Fatalf("iter %d: entry %d = %q; want %q", iter, i, got[i], want[i])
					}
				}
			}
		})
	}
}

// Depth reported by the DirEntry must match the path depth.
func TestWalkStressDepth(t *testing.T) {
	root, _ := buildTree(t)
	for _, workers := range []int{1, 4, 16} {
		err := fastwalk.Walk(&fastwalk.Config{NumWorkers: workers}, root,
			func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				rel, err := filepath.Rel(root, p)
				if err != nil {
					return err
				}
				want := 0
				if rel != "." {
					want = len(filepath.SplitList(rel)) + strings_Count(rel, string(os.PathSeparator))
				}
				if got := fastwalk.DirEntryDepth(d); got != want {
					t.Errorf("workers=%d %q: depth %d; want %d", workers, rel, got, want)
				}
				return nil
			})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func strings_Count(s, sub string) int {
	n := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			n++
		}
	}
	return n
}

// An error from walkFn must stop the walk and be returned, with no callbacks
// running after Walk returns.
func TestWalkStressError(t *testing.T) {
	root, _ := buildTree(t)
	errBoom := errors.New("boom")
	for _, workers := range []int{1, 4, 16} {
		for iter := 0; iter < 50; iter++ {
			var live atomic.Int64
			var after atomic.Bool
			var n atomic.Int64
			var done atomic.Bool
			err := fastwalk.Walk(&fastwalk.Config{NumWorkers: workers}, root,
				func(p string, _ fs.DirEntry, err error) error {
					if done.Load() {
						after.Store(true)
					}
					live.Add(1)
					defer live.Add(-1)
					if err != nil {
						return err
					}
					if n.Add(1) == 25 {
						return errBoom
					}
					return nil
				})
			done.Store(true)
			if !errors.Is(err, errBoom) {
				t.Fatalf("workers=%d iter=%d: err = %v; want %v", workers, iter, err, errBoom)
			}
			if live.Load() != 0 {
				t.Fatalf("workers=%d: %d callbacks still running after Walk returned", workers, live.Load())
			}
			if after.Load() {
				t.Fatalf("workers=%d: callback ran after Walk returned", workers)
			}
		}
	}
}

// Walking many small trees concurrently should not leak goroutines.
func TestWalkStressGoroutineLeak(t *testing.T) {
	root, _ := buildTree(t)
	base := runtime.NumGoroutine()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				err := fastwalk.Walk(&fastwalk.Config{NumWorkers: 8}, root,
					func(string, fs.DirEntry, error) error { return nil })
				if err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	for i := 0; i < 50; i++ {
		if runtime.NumGoroutine() <= base+2 {
			return
		}
		runtime.Gosched()
	}
	t.Errorf("goroutines: %d; started with %d", runtime.NumGoroutine(), base)
}

// Walking a single empty directory must not spawn workers it cannot use.
func TestWalkSmallTree(t *testing.T) {
	root := t.TempDir()
	base := runtime.NumGoroutine()
	var n int
	err := fastwalk.Walk(&fastwalk.Config{NumWorkers: 32}, root,
		func(string, fs.DirEntry, error) error { n++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("visited %d entries; want 1", n)
	}
	if g := runtime.NumGoroutine(); g > base+1 {
		t.Errorf("spawned %d goroutines for an empty directory", g-base)
	}
}
