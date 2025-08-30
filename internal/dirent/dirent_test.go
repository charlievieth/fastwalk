//go:build aix || dragonfly || freebsd || (js && wasm) || linux || netbsd || openbsd || solaris

package dirent

import "testing"

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
