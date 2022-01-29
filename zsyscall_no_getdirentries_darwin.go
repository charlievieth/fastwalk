//go:build nogetdirentries
// +build nogetdirentries

package fastwalk

const useGetdirentries = false

func Getdirentries(fd int, _ []byte, _ *uintptr) (int, error) {
	panic("NOT IMPLEMENTED")
}
