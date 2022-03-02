package fastwalk

import (
	"fmt"
	"io/fs"
	"os"
	"testing"
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

func TestCleanRootPath(t *testing.T) {
	tests := map[string]string{
		"":      "",
		"/":     "/",
		"//":    "/",
		"/foo":  "/foo",
		"/foo/": "/foo",
		"a":     "a",
		`C:/`:   `C:`,
	}
	if os.PathSeparator != '/' {
		const sep = string(os.PathSeparator)
		tests["C:"+sep] = `C:`
		tests["C:"+sep+sep] = `C:`
		tests[sep+sep] = sep
	}
	for in, want := range tests {
		got := cleanRootPath(in)
		if got != want {
			t.Errorf("cleanRootPath(%q) = %q; want: %q", in, got, want)
		}
	}
}
