//go:build linux
// +build linux

package local

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests exercise groupadd, usermod and the sg re-exec against a real
// Linux account database. They change the host's groups, so they only run
// when TL_SHARED_GROUP_INTEGRATION=1 — meant for a throwaway container (see
// MULTI-USER.md, "Testing the group setup"). Member names that must be in the
// group come from TL_SHARED_GROUP_EXPECT_MEMBERS (space separated); names that
// must stay out from TL_SHARED_GROUP_EXPECT_ABSENT. TL_SHARED_GROUP_EXPECT_REEXEC
// asserts that the run was re-executed under sg (the user's shell started
// without the group), and TL_SHARED_GROUP_EXPECT_ERROR that setup fails with
// the actionable error (a non-member who cannot use sudo).
func skipUnlessSharedGroupIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("TL_SHARED_GROUP_INTEGRATION") == "" {
		t.Skip("set TL_SHARED_GROUP_INTEGRATION=1 in a throwaway container to run")
	}
}

func TestSharedGroupEndToEnd(t *testing.T) {
	skipUnlessSharedGroupIntegration(t)

	// May re-exec this test binary under sg; the re-executed run is the one
	// that reaches the assertions below.
	err := EnsureSharedGroup()
	if os.Getenv("TL_SHARED_GROUP_EXPECT_ERROR") != "" {
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sudo usermod -aG "+SHARED_GROUP_NAME)
		t.Logf("expected failure: %v", err)
		return
	}
	require.NoError(t, err)

	gid, ok := SharedGroupGID()
	require.True(t, ok, "group %s should exist", SHARED_GROUP_NAME)
	if os.Geteuid() != 0 {
		assert.True(t, processHasGroup(gid), "process should carry the group after EnsureSharedGroup")
	}
	if os.Getenv("TL_SHARED_GROUP_EXPECT_REEXEC") != "" {
		assert.NotEmpty(t, os.Getenv(sharedGroupReexecEnv), "a user without the group at login must have been re-executed under sg")
	}

	_, members, found := readGroup(SHARED_GROUP_NAME)
	require.True(t, found)
	for _, want := range strings.Fields(os.Getenv("TL_SHARED_GROUP_EXPECT_MEMBERS")) {
		assert.Contains(t, members, want)
	}
	for _, absent := range strings.Fields(os.Getenv("TL_SHARED_GROUP_EXPECT_ABSENT")) {
		assert.NotContains(t, members, absent)
	}
	groups, _ := os.Getgroups()
	t.Logf("group %s gid=%d members=%v; euid=%d egid=%d groups=%v", SHARED_GROUP_NAME, gid, members, os.Geteuid(), os.Getegid(), groups)
}

// TestSharedDataDirEndToEnd runs the real InitStandaloneDir against
// $TL_DATA_DIR as the current user, then replaces a file in manifests/ the way
// the installer records state. Run it as two different members in turn: the
// second run replaces what the first created.
func TestSharedDataDirEndToEnd(t *testing.T) {
	skipUnlessSharedGroupIntegration(t)
	if os.Getenv(DATA_DIR_ENV_NAME) == "" {
		t.Skip("set TL_DATA_DIR to the data dir to exercise")
	}

	require.NoError(t, InitStandaloneDir())

	gid, ok := SharedGroupGID()
	require.True(t, ok)
	dataDir := GetServerDataDir()
	for _, dir := range append([]string{"."}, sharedDataSubDirs...) {
		assertSharedDir(t, filepath.Join(dataDir, dir), gid)
	}
	for _, dir := range storageDataSubDirs {
		info, err := os.Stat(filepath.Join(dataDir, dir))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o777), info.Mode().Perm(), "mode of %s", dir)
		assert.Zero(t, info.Mode()&os.ModeSetgid, "%s must not carry setgid", dir)
	}
	cache, err := os.Stat(filepath.Join(dataDir, CONTAINERD_DIR_NAME))
	require.NoError(t, err)
	assert.Zero(t, cache.Mode()&os.ModeSetgid)

	params := filepath.Join(dataDir, MANIFEST_DIR_NAME, "params.yaml")
	me := currentUserName()
	require.NoError(t, WriteFileAtomic(params, []byte(me+"\n"), 0o664))
	got, err := os.ReadFile(params)
	require.NoError(t, err)
	assert.Equal(t, me+"\n", string(got))
	info, err := os.Stat(params)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o664), info.Mode().Perm())
	assert.Equal(t, gid, statGID(t, params))
	t.Logf("%s wrote %s as uid %d", me, params, os.Geteuid())
}
