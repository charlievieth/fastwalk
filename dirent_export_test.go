package fastwalk

import (
	"fmt"
	"io/fs"
	"os"
	"time"
)

// Export funcs for testing (because I'm too lazy to move the
// symlink() and writeFile() funcs)

func FormatFileInfo(fi fs.FileInfo) string {
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
