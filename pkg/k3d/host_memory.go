package k3d

// Docker Desktop runs the engine in a VM whose guest page cache keeps the whole
// allocation resident on the host, so whatever is left over is all the host OS,
// the browser and this CLI ever get (EN-2556).
const MIN_HOST_MEMORY_HEADROOM_BYTES int64 = 8 * 1024 * 1024 * 1024

func dockerMemoryStarvesHost(hostPhysicalBytes, dockerMemTotal int64) bool {
	if hostPhysicalBytes <= 0 {
		return false
	}
	return hostPhysicalBytes-dockerMemTotal < MIN_HOST_MEMORY_HEADROOM_BYTES
}
