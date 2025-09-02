//go:build aix || dragonfly || freebsd || (js && wasm) || linux || netbsd || openbsd || solaris

package dirent

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"testing"
)

func TestReadInt(t *testing.T) {
	for _, sz := range []int{1, 2, 4, 8} {
		rr := rand.New(rand.NewSource(1))
		t.Run(fmt.Sprintf("%d", sz), func(t *testing.T) {
			var fn func() uint64
			switch sz {
			case 1:
				fn = func() uint64 { return uint64(rr.Int63n(256)) }
			case 2:
				fn = func() uint64 { return uint64(rr.Int63n(math.MaxUint16)) }
			case 4:
				fn = func() uint64 { return uint64(rr.Int63n(math.MaxUint32)) }
			case 8:
				fn = func() uint64 { return uint64(rr.Uint64()) }
			default:
				t.Fatal("invalid size:", sz)
			}
			buf := make([]byte, 8)
			fails := 0
			for i := 0; i < 100; i++ {
				want := fn()
				binary.NativeEndian.PutUint64(buf[:], want)
				got, ok := readInt(buf, 0, uintptr(sz))
				if got != want || !ok {
					fails++
					t.Errorf("readInt(%q, 0, %d) = %d, %t; want: %d, %t", buf, sz, got, ok, want, true)
				}
				if fails >= 10 {
					t.Fatal("too many errors:", fails)
				}
			}
		})
	}
	if i, ok := readInt(nil, 1, 1); i != 0 || ok {
		t.Errorf("readInt(nil, 1, 1) = %d, %t; want: %d, %t", i, ok, 0, false)
	}
}

func TestReadIntSize(t *testing.T) {
	if i, ok := readInt(nil, 1, 1); i != 0 || ok {
		t.Errorf("readInt(nil, 1, 1) = %d, %t; want: %d, %t", i, ok, 0, false)
	}
}

func TestReadIntInvalidSize(t *testing.T) {
	defer func() {
		if e := recover(); e == nil {
			t.Errorf("Expected panic for invalid size")
		}
	}()
	readInt(make([]byte, 32), 0, 9)
}
