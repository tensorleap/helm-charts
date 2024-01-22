package version

import (
	"strconv"
	"strings"
)

const Version = "v0.0.8"

func IsMinorVersionSmaller(currentVersion, comperedVersion string) bool {

	currentVersions := SplitVersion(currentVersion)
	comperedVersions := SplitVersion(comperedVersion)

	return currentVersions[0] > comperedVersions[0] || currentVersions[1] > comperedVersions[1]
}

func IsMinorVersionChange(currentVersion, comperedVersion string) bool {
	currentVersions := SplitVersion(currentVersion)
	comperedVersions := SplitVersion(comperedVersion)

	return currentVersions[0] != comperedVersions[0] || currentVersions[1] != comperedVersions[1]
}

func SplitVersion(version string) []uint {
	versionNumbers := strings.Split(strings.TrimPrefix(version, "v"), ".")[:3]
	result := make([]uint, len(versionNumbers))
	for i, v := range versionNumbers {
		num, _ := strconv.ParseInt(v, 10, 64)
		result[i] = uint(num)
	}
	return result
}
