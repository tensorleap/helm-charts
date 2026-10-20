package k3d

import "testing"

const gib int64 = 1024 * 1024 * 1024

func TestDockerMemoryStarvesHost(t *testing.T) {
	cases := []struct {
		name     string
		physical int64
		docker   int64
		want     bool
	}{
		// EN-2556: 36 GiB MacBook Pro, Docker VM reporting 35.66 GB MemTotal.
		{"tom's laptop leaves under 3 GiB", 36 * gib, 35659309056, true},
		{"64 GiB host with 48 GiB docker", 64 * gib, 48 * gib, false},
		{"exactly the minimum headroom is fine", 36 * gib, 28 * gib, false},
		{"one byte under the minimum fires", 36 * gib, 28*gib + 1, true},
		{"unknown physical memory never fires", 0, 34 * gib, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := dockerMemoryStarvesHost(c.physical, c.docker); got != c.want {
				t.Fatalf("dockerMemoryStarvesHost(%d, %d) = %v, want %v", c.physical, c.docker, got, c.want)
			}
		})
	}
}
