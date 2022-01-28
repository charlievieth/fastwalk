//go:build (linux && !appengine) || darwin || freebsd || openbsd || netbsd || !windows
// +build linux,!appengine darwin freebsd openbsd netbsd !windows

package fastwalk

import (
	"math/rand"
	"runtime"
	"sync"
	"testing"
	"time"
)

type devIno struct {
	Dev, Ino uint64
}

func generateDevIno(rr *rand.Rand, ndev, size int) []devIno {
	devs := make([]uint64, ndev)
	for i := range devs {
		devs[i] = rr.Uint64()
	}
	pairs := make([]devIno, size)
	seen := make(map[devIno]struct{}, len(pairs))
	for i := range pairs {
		for {
			di := devIno{
				Dev: devs[rr.Intn(len(devs))],
				Ino: rr.Uint64(),
			}
			if _, ok := seen[di]; !ok {
				pairs[i] = di
				seen[di] = struct{}{}
				break
			}
		}
	}
	rr.Shuffle(len(pairs), func(i, j int) {
		pairs[i], pairs[j] = pairs[j], pairs[i]
	})
	return pairs
}

func TestEntryFilterFiles(t *testing.T) {
	rr := rand.New(rand.NewSource(1))
	pairs := generateDevIno(rr, 2, 100)

	x := NewEntryFilter()
	for _, p := range pairs {
		if x.seen(p.Dev, p.Ino) {
			t.Errorf("duplicate: Dev: %d Ino: %d", p.Dev, p.Ino)
		}
	}
	for _, p := range pairs {
		if !x.seen(p.Dev, p.Ino) {
			t.Errorf("wat: Dev: %d Ino: %d", p.Dev, p.Ino)
		}
	}
}

func TestEntryFilterFiles_Parallel(t *testing.T) {
	if testing.Short() {
		t.Skip("Short test")
	}
	wg := new(sync.WaitGroup)
	ready := new(sync.WaitGroup)
	start := make(chan struct{})
	x := NewEntryFilter()

	numWorkers := runtime.NumCPU() * 2
	if numWorkers < 2 {
		numWorkers = 2
	}
	if numWorkers > 8 {
		numWorkers = 8
	}

	rr := rand.New(rand.NewSource(time.Now().UnixNano()))
	pairs := generateDevIno(rr, 2, numWorkers*8192)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		ready.Add(1)
		go func(pairs []devIno) {
			defer wg.Done()
			ready.Done()
			<-start
			for _, p := range pairs {
				if x.seen(p.Dev, p.Ino) {
					t.Errorf("%d: unseen dev/ino: Dev: %d Ino: %d", i, p.Dev, p.Ino)
					return
				}
			}
			for _, p := range pairs {
				if !x.seen(p.Dev, p.Ino) {
					t.Errorf("%d: missed seen dev/ino: Dev: %d Ino: %d", i, p.Dev, p.Ino)
					return
				}
			}
		}(pairs[i*numWorkers : (i+1)*numWorkers])
	}

	ready.Wait()
	close(start)
	wg.Wait()
}

// TODO: delete?
func BenchmarkEntryFilter_Seen(b *testing.B) {
	rr := rand.New(rand.NewSource(1))
	// pairs := GenerateDevIno(rr, 2, 8192)
	pairs := generateDevIno(rr, 1, 8192)
	x := NewEntryFilter()

	for _, p := range pairs {
		x.seen(p.Dev, p.Ino)
	}
	if len(pairs) != 8192 {
		panic("nope!")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := pairs[i%8192]
		x.seen(p.Dev, p.Ino)
	}
}

// TODO: delete?
func BenchmarkEntryFilter_Seen_Parallel(b *testing.B) {
	rr := rand.New(rand.NewSource(1))
	// pairs := GenerateDevIno(rr, 2, 8192)
	pairs := generateDevIno(rr, 1, 8192)
	x := NewEntryFilter()

	for _, p := range pairs {
		x.seen(p.Dev, p.Ino)
	}
	if len(pairs) != 8192 {
		panic("nope!")
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for i := 0; pb.Next(); i++ {
			p := pairs[i%8192]
			x.seen(p.Dev, p.Ino)
		}
	})
}

// func BenchmarkMap_Seen(b *testing.B) {
// 	const N = 8192
// 	// const N = 1024 * 1024
// 	rr := rand.New(rand.NewSource(1))
// 	// pairs := GenerateDevIno(rr, 2, 8192)
// 	// pairs := GenerateDevIno(rr, 1, 8192)
// 	pairs := GenerateDevIno(rr, 1, N)
// 	var mu sync.Mutex
// 	seen := make(map[DevIno]struct{}, N)
// 	// seen := make(map[uint64]struct{})
//
// 	for _, p := range pairs {
// 		seen[p] = struct{}{}
// 	}
// 	if len(pairs) != N {
// 		panic("nope!")
// 	}
//
// 	b.ResetTimer()
// 	for i := 0; i < b.N; i++ {
// 		p := pairs[i%N]
// 		if _, ok := seen[p]; !ok {
// 			mu.Lock()
// 			b.Fatal("WAT")
// 			mu.Unlock()
// 		}
// 	}
// }
