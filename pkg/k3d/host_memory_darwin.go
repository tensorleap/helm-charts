package k3d

import "golang.org/x/sys/unix"

func hostPhysicalMemoryBytes() (int64, bool) {
	size, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return 0, false
	}
	return int64(size), true
}
