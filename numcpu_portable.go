//go:build !(darwin && arm64)

package fastwalk

func darwinNumPerfCores() int {
	return -1
}
