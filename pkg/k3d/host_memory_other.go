//go:build !darwin

package k3d

// Only Docker Desktop on macOS pins a fixed VM allocation against the host;
// on Linux docker shares the host kernel and MemTotal is the machine itself.
func hostPhysicalMemoryBytes() (int64, bool) {
	return 0, false
}
