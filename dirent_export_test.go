package fastwalk

import (
	"fmt"
	"io/fs"
	"os"
	"runtime"
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

// NB: this test lives here and not in fastwalk_test.go since we need to
// access the internal cleanRootPath function.
func TestCleanRootPath(t *testing.T) {
	test := func(t *testing.T, tests map[string]string) {
		t.Helper()
		for in, want := range tests {
			got := cleanRootPath(in)
			if got != want {
				t.Errorf("cleanRootPath(%q) = %q; want: %q", in, got, want)
			}
		}
	}
	// NB: The name here isn't exactly correct since we run this for
	// any non-Windows OS.
	t.Run("Unix", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("test not supported on Windows")
		}
		test(t, map[string]string{
			"":      "",
			".":     ".",
			"/":     "/",
			"//":    "/",
			"/foo":  "/foo",
			"/foo/": "/foo",
			"a":     "a",
		})
	})
	// Test that cleanRootPath is a no-op on Windows
	t.Run("Windows", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("test only supported on Windows")
		}
		test(t, map[string]string{
			`C:/`:              `C:/`,
			`C://`:             `C://`,
			`\\?\GLOBALROOT`:   `\\?\GLOBALROOT`,
			`\\?\GLOBALROOT\\`: `\\?\GLOBALROOT\\`,
		})
	})
}
