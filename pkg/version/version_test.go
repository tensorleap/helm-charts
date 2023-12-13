package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsMinorVersionChange(t *testing.T) {
	assert.False(t, IsMinorVersionChange("v0.0.1", "v0.0.2"))
	assert.False(t, IsMinorVersionChange("0.0.1", "0.0.2"))
	assert.False(t, IsMinorVersionChange("0.0.1-beta.0", "0.0.2-beta.0"))
	assert.True(t, IsMinorVersionChange("0.0.1", "0.1.0"))
	assert.True(t, IsMinorVersionChange("0.0.1-beta.0", "0.1.0-beta.0"))

}

func TestIsMinorVersionSmaller(t *testing.T) {
	assert.False(t, IsMinorVersionSmaller("0.0.1", "0.0.2"))
	assert.False(t, IsMinorVersionSmaller("v0.0.1", "v0.0.2"))
	assert.False(t, IsMinorVersionSmaller("v0.0.1", "v0.1.0"))
	assert.False(t, IsMinorVersionSmaller("v0.0.1-beta.0", "v0.1.0-beta.0"))

	assert.True(t, IsMinorVersionSmaller("v3.0.3", "v2.0.2"))
	assert.True(t, IsMinorVersionSmaller("v0.2.1", "v0.1.0"))
	assert.True(t, IsMinorVersionSmaller("v0.2.1-beta.0", "v0.1.0-beta.0"))
}
