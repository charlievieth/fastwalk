//go:build !windows

package fastwalk

func useForwardSlash() bool {
	return false
}
