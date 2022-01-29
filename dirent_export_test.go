package fastwalk

import (
	"io/fs"
	"os"
)

// Export funcs for testing (because I'm too lazy to move the
// symlink() and writeFile() funcs)

func LstatDirent(path string, d fs.DirEntry) (os.FileInfo, error) {
	return lstatDirent(path, d)
}

func StatDirent(path string, d fs.DirEntry) (os.FileInfo, error) {
	return statDirent(path, d)
}
