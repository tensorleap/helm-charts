//go:build !windows
// +build !windows

package local

import (
	"io/fs"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// fakeFileInfo lets the permission rule be checked for owners and groups the
// test process is not, which real files cannot express without root.
type fakeFileInfo struct {
	mode os.FileMode
	stat syscall.Stat_t
}

func (f fakeFileInfo) Name() string       { return "fake" }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeFileInfo) Sys() any           { return &f.stat }

var _ fs.FileInfo = fakeFileInfo{}

func TestSetPermissionFromFileInfo(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permission bits")
	}
	me := uint32(os.Geteuid())
	myGroup := uint32(os.Getegid())
	const nobody, noGroup = 60001, 60002 // neither ours

	tests := []struct {
		name      string
		mode      os.FileMode
		uid, gid  uint32
		wantWrite bool
		wantRead  bool
	}{
		{"owner bits apply to the owner", 0o700, me, noGroup, true, true},
		{"owner bits only, even if group would allow", 0o070, me, myGroup, false, false},
		{"group bits apply to a member", 0o070, nobody, myGroup, true, true},
		{"group bits do not apply to a non-member", 0o070, nobody, noGroup, false, false},
		{"other bits apply to everyone else", 0o007, nobody, noGroup, true, true},
		{"read-only for a member", 0o050, nobody, myGroup, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := fakeFileInfo{mode: tt.mode | os.ModeDir, stat: syscall.Stat_t{Uid: tt.uid, Gid: tt.gid}}
			var status FileSystemStatus
			assert.NoError(t, SetPermissionFromFileInfo(&status, info))
			assert.Equal(t, tt.wantWrite, status.CanWrite, "CanWrite")
			assert.Equal(t, tt.wantRead, status.CanRead, "CanRead")
		})
	}

	t.Run("supplementary groups count", func(t *testing.T) {
		groups, err := os.Getgroups()
		if err != nil {
			t.Skip("no supplementary groups")
		}
		for _, g := range groups {
			if g == os.Getegid() {
				continue
			}
			info := fakeFileInfo{mode: 0o070 | os.ModeDir, stat: syscall.Stat_t{Uid: nobody, Gid: uint32(g)}}
			var status FileSystemStatus
			assert.NoError(t, SetPermissionFromFileInfo(&status, info))
			assert.True(t, status.CanWrite, "gid %d is one of ours", g)
			return
		}
		t.Skip("no supplementary group other than the effective one")
	})
}
