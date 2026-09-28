package local

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePasswd(t *testing.T) {
	input := strings.Join([]string{
		"# a comment",
		"root:x:0:0:root:/root:/bin/bash",
		"ubuntu:x:1000:1000:Ubuntu:/home/ubuntu:/bin/bash",
		"broken:x:notanumber:1:::/bin/sh",
		"short:x:1",
		"+@netgroup",
		"",
		"ssm-user:x:1001:1001::/home/ssm-user:/bin/bash",
	}, "\n")

	got := parsePasswd(strings.NewReader(input))

	assert.Equal(t, []localAccount{
		{Name: "root", UID: 0, GID: 0, Shell: "/bin/bash"},
		{Name: "ubuntu", UID: 1000, GID: 1000, Shell: "/bin/bash"},
		{Name: "ssm-user", UID: 1001, GID: 1001, Shell: "/bin/bash"},
	}, got)
}

func TestParseLoginDefsUIDRange(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantMin int
		wantMax int
	}{
		{"defaults when unset", "MAIL_DIR /var/mail\n", defaultUIDMin, defaultUIDMax},
		{"both set", "UID_MIN\t\t 500\nUID_MAX\t\t 29999\n", 500, 29999},
		{"commented lines ignored", "#UID_MIN 1\nUID_MIN 2000\n", 2000, defaultUIDMax},
		{"garbage ignored", "UID_MIN lots\n", defaultUIDMin, defaultUIDMax},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMin, gotMax := parseLoginDefsUIDRange(strings.NewReader(tt.input))
			assert.Equal(t, tt.wantMin, gotMin)
			assert.Equal(t, tt.wantMax, gotMax)
		})
	}
}

func TestParseGroup(t *testing.T) {
	input := "root:x:0:\ndocker:x:998:ubuntu,ssm-user\ntensorleap:x:997: ubuntu ,bob\n"
	tests := []struct {
		name        string
		group       string
		wantGID     int
		wantMembers []string
		wantFound   bool
	}{
		{"members listed", "docker", 998, []string{"ubuntu", "ssm-user"}, true},
		{"members trimmed", "tensorleap", 997, []string{"ubuntu", "bob"}, true},
		{"no members", "root", 0, nil, true},
		{"missing group", "nope", 0, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gid, members, found := parseGroup(strings.NewReader(input), tt.group)
			assert.Equal(t, tt.wantFound, found)
			assert.Equal(t, tt.wantGID, gid)
			assert.Equal(t, tt.wantMembers, members)
		})
	}
}

func TestIsHumanAccount(t *testing.T) {
	tests := []struct {
		name    string
		account localAccount
		want    bool
	}{
		{"login user", localAccount{Name: "ubuntu", UID: 1000, Shell: "/bin/bash"}, true},
		{"top of range", localAccount{Name: "last", UID: 60000, Shell: "/usr/bin/zsh"}, true},
		{"system account", localAccount{Name: "daemon", UID: 1, Shell: "/usr/sbin/nologin"}, false},
		{"system account with a shell", localAccount{Name: "svc", UID: 999, Shell: "/bin/bash"}, false},
		{"service account in range without a shell", localAccount{Name: "svc", UID: 1005, Shell: "/usr/sbin/nologin"}, false},
		{"false shell", localAccount{Name: "svc", UID: 1005, Shell: "/bin/false"}, false},
		{"empty shell", localAccount{Name: "svc", UID: 1005, Shell: ""}, false},
		{"above range", localAccount{Name: "nobody", UID: 65534, Shell: "/bin/sh"}, false},
		{"ssm-user always", localAccount{Name: "ssm-user", UID: 65000, Shell: "/usr/sbin/nologin"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isHumanAccount(tt.account, defaultUIDMin, defaultUIDMax))
		})
	}
}

func TestSharedGroupMembersToAdd(t *testing.T) {
	const gid = 997
	accounts := []localAccount{
		{Name: "root", UID: 0, GID: 0, Shell: "/bin/bash"},
		{Name: "daemon", UID: 1, GID: 1, Shell: "/usr/sbin/nologin"},
		{Name: "ubuntu", UID: 1000, GID: 1000, Shell: "/bin/bash"},
		{Name: "svc", UID: 1001, GID: 1001, Shell: "/usr/sbin/nologin"},
		{Name: "ssm-user", UID: 1002, GID: 1002, Shell: "/bin/bash"},
		{Name: "alice", UID: 1003, GID: gid, Shell: "/bin/bash"},
		{Name: "carol", UID: 1004, GID: 1004, Shell: "/bin/zsh"},
		{Name: "toor", UID: 0, GID: 0, Shell: "/bin/bash"},
	}
	members := []string{"ubuntu"}

	got := sharedGroupMembersToAdd(accounts, defaultUIDMin, defaultUIDMax, gid, members,
		"bob", "carol", "", "bad name", "bob", "root", "toor")

	// ubuntu is already a member, alice has it as primary group, svc and daemon
	// are not people, "bad name" is not a valid account name, root and the
	// uid-0 alias toor never need the group, bob (from NSS, not in passwd) is
	// added once.
	assert.Equal(t, []string{"ssm-user", "carol", "bob"}, got)
}

func TestShellJoin(t *testing.T) {
	got := shellJoin([]string{"/usr/local/bin/leap", "server", "install", "--data-dir", "/mnt/it's here"})
	assert.Equal(t, `'/usr/local/bin/leap' 'server' 'install' '--data-dir' '/mnt/it'\''s here'`, got)
}

func TestProcessHasGroup(t *testing.T) {
	assert.True(t, processHasGroup(os.Getegid()), "the effective group always counts")
	assert.False(t, processHasGroup(-12345))
}

func TestSharedGroupGIDIsAbsentOffLinux(t *testing.T) {
	// Sanity check of the default lookup: only Linux hosts with the group
	// installed report one, so on a dev machine or CI runner without the
	// group the policies must fall back.
	_, ok := SharedGroupGID()
	if ok {
		t.Skip("the tensorleap group exists on this host")
	}
	require.False(t, ok)
}

func TestActivateSharedGroupNoGroupIsNoop(t *testing.T) {
	// With no shared group on the host (the common case off an installed Linux
	// box: mac dev, CI, a machine before first install), ActivateSharedGroup
	// must do nothing and never prompt or error.
	prev := lookupSharedGroupGID
	lookupSharedGroupGID = func() (int, bool) { return -1, false }
	t.Cleanup(func() { lookupSharedGroupGID = prev })
	ActivateSharedGroup() // must not panic, prompt, or re-exec
}

func TestActivateSharedGroupAlreadyMemberIsNoop(t *testing.T) {
	// When the process already carries the group, ActivateSharedGroup returns
	// without touching sudo or re-executing.
	skipOnWindows(t)
	prev := lookupSharedGroupGID
	lookupSharedGroupGID = func() (int, bool) { return os.Getegid(), true }
	t.Cleanup(func() { lookupSharedGroupGID = prev })
	ActivateSharedGroup() // effective gid == "shared" gid => processHasGroup true => no-op
}
