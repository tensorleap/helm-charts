package local

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/tensorleap/helm-charts/pkg/log"
)

// SHARED_GROUP_NAME is the local Unix group that owns the directories humans
// write to in the Tensorleap data dir. Every human account on the host is made
// a member on every run, so whoever installed, upgrades or operates the same
// single-node install (ubuntu installs, ssm-user upgrades, a colleague runs
// `leap server stop`). See MULTI-USER.md.
const SHARED_GROUP_NAME = "tensorleap"

const (
	passwdFile    = "/etc/passwd"
	groupFile     = "/etc/group"
	loginDefsFile = "/etc/login.defs"
	// AWS Session Manager creates this account on first use. It falls in the
	// human UID range with a bash shell, but is listed explicitly so a
	// customised login.defs cannot exclude it.
	ssmUserName = "ssm-user"
	// shadow-utils defaults, used when login.defs does not set them.
	defaultUIDMin = 1000
	defaultUIDMax = 60000
)

// validUserName is the portable useradd name syntax plus the trailing $ of
// Samba machine accounts. Names only ever travel as separate argv entries,
// never through a shell, so this is defence in depth rather than the only guard.
var validUserName = regexp.MustCompile(`^[a-zA-Z0-9_.][a-zA-Z0-9_.@-]{0,31}\$?$`)

// lookupSharedGroupGID resolves the shared group; tests override it so the
// policies can be exercised on hosts where the group does not exist.
var lookupSharedGroupGID = func() (int, bool) {
	if runtime.GOOS != "linux" {
		return -1, false
	}
	g, err := user.LookupGroup(SHARED_GROUP_NAME)
	if err != nil {
		return -1, false
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return -1, false
	}
	return gid, true
}

// SharedGroupGID returns the gid of the shared group when it exists on this
// host. It is absent on non-Linux hosts and before the first install.
func SharedGroupGID() (int, bool) { return lookupSharedGroupGID() }

// EnsureSharedGroup makes the shared group exist with every human account in
// it and makes the current process carry it. Linux only; a no-op elsewhere.
//
// Group membership is read at login, so a user added on this run does not
// carry the group yet. Rather than asking them to log out and back in, the
// process re-executes the same command under sg(1), which starts it with the
// group active; later logins have it natively. Root never needs the group.
//
// The returned error is advisory: callers warn and continue, because the
// owning user can still work in directories they created, and failing would
// block single-user hosts that work today.
func EnsureSharedGroup() error {
	if runtime.GOOS != "linux" {
		return nil
	}
	gid, err := ensureSharedGroupExists()
	if err != nil {
		return err
	}
	if err := syncSharedGroupMembers(gid); err != nil {
		log.Warnf("Could not add every local user to group %s: %v", SHARED_GROUP_NAME, err)
	}
	if os.Geteuid() == 0 || processHasGroup(gid) {
		return nil
	}
	current := currentUserName()
	if !isSharedGroupMember(current, gid) {
		return fmt.Errorf("user %s is not a member of group %s; an admin can fix it with: sudo usermod -aG %s %s (then log out and back in)",
			current, SHARED_GROUP_NAME, SHARED_GROUP_NAME, current)
	}
	log.Infof("Activating group %s for this run", SHARED_GROUP_NAME)
	return reexecWithSharedGroup()
}

func ensureSharedGroupExists() (int, error) {
	if gid, ok := lookupSharedGroupGID(); ok {
		return gid, nil
	}
	log.Printf("Creating group %s (you may be asked to enter the root user password)", SHARED_GROUP_NAME)
	if err := runAsRoot("groupadd", "--system", SHARED_GROUP_NAME); err != nil {
		return -1, fmt.Errorf("failed to create group %s: %w", SHARED_GROUP_NAME, err)
	}
	gid, ok := lookupSharedGroupGID()
	if !ok {
		return -1, fmt.Errorf("group %s not found after creating it", SHARED_GROUP_NAME)
	}
	return gid, nil
}

// syncSharedGroupMembers adds every human account that is not a member yet.
// It runs on every command so accounts created after the install (ssm-user
// appears on the first Session Manager login) get in on the next run.
func syncSharedGroupMembers(gid int) error {
	toAdd, err := sharedGroupMembersToAddFromHost(gid)
	if err != nil {
		return err
	}
	if len(toAdd) == 0 {
		return nil
	}
	log.Printf("Adding %s to group %s", strings.Join(toAdd, ", "), SHARED_GROUP_NAME)
	var firstErr error
	for _, name := range toAdd {
		// Non-interactive: adding another account is done on their behalf, so a
		// caller without cached sudo credentials is warned, never prompted.
		if err := runAsRootQuiet("usermod", "-aG", SHARED_GROUP_NAME, name); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// ActivateSharedGroup makes the current process carry the shared group for a
// read-only command such as `leap server tools kubectl`, when an install
// already exists. Unlike EnsureSharedGroup it never creates the group and never
// prompts for a password: a missing group means there is no install to join, so
// it does nothing; it best-effort adds accounts with `sudo -n`; and it re-execs
// under sg only when that made the caller a member. It never hard-fails — the
// read command still falls through to its own behaviour if group access turns
// out to be genuinely unavailable.
//
// This is what lets a brand-new account (an ssm-user the SSM agent created after
// the install) reach the group-readable shared kubeconfig on its very first
// command, without a prior lifecycle command or a re-login.
func ActivateSharedGroup() {
	if runtime.GOOS != "linux" {
		return
	}
	gid, ok := lookupSharedGroupGID()
	if !ok {
		return // no group yet => no install to join
	}
	if os.Geteuid() == 0 || processHasGroup(gid) {
		return // root, or already carrying the group => nothing to do
	}
	if err := syncSharedGroupMembers(gid); err != nil {
		log.Warnf("Could not sync group %s membership: %v", SHARED_GROUP_NAME, err)
	}
	if !isSharedGroupMember(currentUserName(), gid) {
		log.Warnf("%s is not a member of group %s; shared files such as the kubeconfig may be unreadable. An admin can add you: sudo usermod -aG %s %s (then log out and back in)",
			currentUserName(), SHARED_GROUP_NAME, SHARED_GROUP_NAME, currentUserName())
		return
	}
	log.Infof("Activating group %s for this run", SHARED_GROUP_NAME)
	if err := reexecWithSharedGroup(); err != nil {
		log.Warnf("Could not activate group %s for this run: %v", SHARED_GROUP_NAME, err)
	}
}

func sharedGroupMembersToAddFromHost(gid int) ([]string, error) {
	accounts, err := readPasswd()
	if err != nil {
		return nil, err
	}
	uidMin, uidMax := readLoginDefsUIDRange()
	_, members, _ := readGroup(SHARED_GROUP_NAME)
	extra := []string{currentUserName(), os.Getenv("SUDO_USER")}
	return sharedGroupMembersToAdd(accounts, uidMin, uidMax, gid, members, extra...), nil
}

// localAccount is one passwd(5) record.
type localAccount struct {
	Name  string
	UID   int
	GID   int
	Shell string
}

func readPasswd() ([]localAccount, error) {
	f, err := os.Open(passwdFile)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", passwdFile, err)
	}
	defer func() { _ = f.Close() }()
	return parsePasswd(f), nil
}

// parsePasswd parses passwd(5) records; malformed and NIS marker lines are skipped.
func parsePasswd(r io.Reader) []localAccount {
	var accounts []localAccount
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
			continue
		}
		f := strings.Split(line, ":")
		if len(f) < 7 {
			continue
		}
		uid, err := strconv.Atoi(f[2])
		if err != nil {
			continue
		}
		gid, err := strconv.Atoi(f[3])
		if err != nil {
			continue
		}
		accounts = append(accounts, localAccount{Name: f[0], UID: uid, GID: gid, Shell: f[6]})
	}
	return accounts
}

func readLoginDefsUIDRange() (uidMin, uidMax int) {
	f, err := os.Open(loginDefsFile)
	if err != nil {
		return defaultUIDMin, defaultUIDMax
	}
	defer func() { _ = f.Close() }()
	return parseLoginDefsUIDRange(f)
}

// parseLoginDefsUIDRange returns UID_MIN and UID_MAX from login.defs(5), with
// the shadow-utils defaults for whichever is unset.
func parseLoginDefsUIDRange(r io.Reader) (uidMin, uidMax int) {
	uidMin, uidMax = defaultUIDMin, defaultUIDMax
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		v, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		switch fields[0] {
		case "UID_MIN":
			uidMin = v
		case "UID_MAX":
			uidMax = v
		}
	}
	return uidMin, uidMax
}

func readGroup(name string) (gid int, members []string, found bool) {
	f, err := os.Open(groupFile)
	if err != nil {
		return 0, nil, false
	}
	defer func() { _ = f.Close() }()
	return parseGroup(f, name)
}

// parseGroup returns the gid and explicit member list of group name from group(5).
func parseGroup(r io.Reader, name string) (gid int, members []string, found bool) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Split(strings.TrimSpace(sc.Text()), ":")
		if len(f) < 4 || f[0] != name {
			continue
		}
		gid, err := strconv.Atoi(f[2])
		if err != nil {
			continue
		}
		for _, m := range strings.Split(f[3], ",") {
			if m = strings.TrimSpace(m); m != "" {
				members = append(members, m)
			}
		}
		return gid, members, true
	}
	return 0, nil, false
}

// isHumanAccount reports whether a person logs in with this account: a UID in
// the login.defs range with an interactive shell. ssm-user always qualifies.
func isHumanAccount(a localAccount, uidMin, uidMax int) bool {
	if a.Name == ssmUserName {
		return true
	}
	if a.UID < uidMin || a.UID > uidMax {
		return false
	}
	switch filepath.Base(a.Shell) {
	case "nologin", "false", "", ".":
		return false
	}
	return true
}

// sharedGroupMembersToAdd returns the accounts that belong in the group but are
// not in it: every human account plus the extra names (the invoking user and
// $SUDO_USER, which may come from NSS rather than /etc/passwd), minus current
// members, accounts whose primary group already is the shared group, and root,
// which never needs it.
func sharedGroupMembersToAdd(accounts []localAccount, uidMin, uidMax, gid int, members []string, extra ...string) []string {
	present := map[string]bool{}
	for _, m := range members {
		present[m] = true
	}
	uidByName := map[string]int{}
	for _, a := range accounts {
		uidByName[a.Name] = a.UID
		if a.GID == gid {
			present[a.Name] = true
		}
	}
	seen := map[string]bool{}
	var add []string
	consider := func(name string) {
		if name == "" || name == "root" || !validUserName.MatchString(name) || present[name] || seen[name] {
			return
		}
		if uid, known := uidByName[name]; known && uid == 0 {
			return
		}
		seen[name] = true
		add = append(add, name)
	}
	for _, a := range accounts {
		if isHumanAccount(a, uidMin, uidMax) {
			consider(a.Name)
		}
	}
	for _, e := range extra {
		consider(e)
	}
	return add
}

func currentUserName() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return os.Getenv("USER")
}

// isSharedGroupMember reports whether name is in the group per the host's
// account database: listed as a member, or has it as primary group.
func isSharedGroupMember(name string, gid int) bool {
	if _, members, found := readGroup(SHARED_GROUP_NAME); found {
		for _, m := range members {
			if m == name {
				return true
			}
		}
	}
	if u, err := user.Lookup(name); err == nil {
		if primary, err := strconv.Atoi(u.Gid); err == nil && primary == gid {
			return true
		}
	}
	return false
}

// processHasGroup reports whether the current process carries gid as its
// effective or one of its supplementary groups — what the kernel checks, as
// opposed to what /etc/group says.
func processHasGroup(gid int) bool {
	if os.Getegid() == gid {
		return true
	}
	groups, err := os.Getgroups()
	if err != nil {
		return false
	}
	for _, g := range groups {
		if g == gid {
			return true
		}
	}
	return false
}

// shellJoin quotes args for `sh -c`, which is how sg(1) runs its command.
func shellJoin(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(quoted, " ")
}
