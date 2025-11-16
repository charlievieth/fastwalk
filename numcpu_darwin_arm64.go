//go:build darwin && arm64

package fastwalk

import "syscall"

// darwinNumPerfCores returns the number of performance cores currently
// available on macOS/arm64.
func darwinNumPerfCores() int {
	// "hw.physicalcpu" is the number of physical processors available in
	// the current power management mode.
	//
	// https://github.com/apple-oss-distributions/xnu/blob/43a90889846e00bfb5cf1d255cdc0a701a1e05a4/bsd/sys/sysctl.h#L1244
	//
	// NB: We do not cache this value since it could change with the power
	// management mode.
	//
	// TODO: Find someone with a Mac Studio or MacPro to see if this still
	// holds true when the CPU is basically two "fused" ones.
	n, err := syscall.SysctlUint32("hw.perflevel0.physicalcpu")
	if err != nil {
		return -1
	}
	return int(n)
}
