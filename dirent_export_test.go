package fastwalk

import (
	"fmt"
	"io/fs"
	"os"
	"time"
)

// Export funcs for testing (because I'm too lazy to move the
// symlink() and writeFile() funcs)

func LstatDirent(path string, d fs.DirEntry) (os.FileInfo, error) {
	return lstatDirent(path, d)
}

func StatDirent(path string, d fs.DirEntry) (os.FileInfo, error) {
	return statDirent(path, d)
}

func FormatFileInfo(fi os.FileInfo) string {
	return fmt.Sprintf("%+v", struct {
		Name    string
		Size    int64
		Mode    os.FileMode
		ModTime time.Time
		IsDir   bool
		Sys     string
	}{
		Name:    fi.Name(),
		Size:    fi.Size(),
		Mode:    fi.Mode(),
		ModTime: fi.ModTime(),
		IsDir:   fi.IsDir(),
		Sys:     fmt.Sprintf("%+v", fi.Sys()),
	})
}
