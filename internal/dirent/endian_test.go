package dirent

import (
	"strconv"
	"testing"
)

func TestReadInt(t *testing.T) {
	t.Run("Size", func(t *testing.T) {
		if i, ok := readInt(nil, 1, 1); i != 0 || ok {
			t.Errorf("readInt(nil, 1, 1) = %d, %t; want: %d, %t", i, ok, 0, false)
		}
	})

	base := make([]byte, 8)
	encoder.PutUint64(base, 1<<63-1)
	for sz := 1; sz <= 8; sz <<= 1 {
		t.Run(strconv.Itoa(sz), func(t *testing.T) {
			b := make([]byte, 8)
			copy(b[:sz], base)
			want := encoder.Uint64(b)
			got, ok := readInt(b, 0, uintptr(sz))
			if !ok || got != want {
				t.Errorf("readInt(%q, %d, %d) = %d, %t want: %d, %t",
					b, 0, sz, got, ok, want, true)
			}
		})
	}

	t.Run("InvalidSize", func(t *testing.T) {
		defer func() {
			if e := recover(); e == nil {
				t.Errorf("Expected panic for invalid size")
			}
		}()
		readInt(make([]byte, 32), 0, 9)
	})
}
