//go:build windows

package fastwalk

import (
	"bytes"
	"os"
	"runtime"
	"sync"
)

func useForwardSlash() bool {
	// Use a forward slash as the path separator if this a Windows executable
	// running in either MSYS/MSYS2 or WSL.
	return runningUnderMSYS() || runningUnderWSL()
}

// runningUnderMSYS reports if we're running in a MSYS/MSYS2 enviroment.
//
// See: https://github.com/sharkdp/fd/pull/730
func runningUnderMSYS() bool {
	switch os.Getenv("MSYSTEM") {
	case "MINGW64", "MINGW32", "MSYS":
		return true
	}
	return false
}

var underWSL struct {
	once sync.Once
	wsl  bool
}

// runningUnderWSL returns if we're a Widows executable running in WSL.
// See [DefaultToSlash] for an explanation of the heuristics used here.
func runningUnderWSL() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	w := &underWSL
	w.once.Do(func() {
		w.wsl = func() bool {
			// Best check (but not super fast)
			if _, err := os.Lstat("/proc/sys/fs/binfmt_misc/WSLInterop"); err == nil {
				return true
			}
			// Fast check, but could provide a false positive if the user sets
			// this on the Windows side.
			if os.Getenv("WSL_DISTRO_NAME") != "" {
				return true
			}
			// If the binary is compiled for Windows and we're running under Linux
			// then honestly just the presence of "/proc/version" should be enough
			// to determine that we're running under WSL, but check the version
			// string just to be pedantic.
			data, _ := os.ReadFile("/proc/version")
			return bytes.Contains(data, []byte("microsoft")) ||
				bytes.Contains(data, []byte("Microsoft"))
		}()
	})
	return w.wsl
}
