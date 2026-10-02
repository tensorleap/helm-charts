package local

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tensorleap/helm-charts/pkg/log"
)

// overlaySnapshotsSubdir is where containerd's default snapshotter unpacks the
// image layers, relative to the containerd root.
const overlaySnapshotsSubdir = "io.containerd.snapshotter.v1.overlayfs/snapshots"

// ownerOf reports who owns a file; tests swap it to simulate ownership.
var ownerOf = fileOwner

// ImageCacheReowned reports whether the containerd image cache under
// containerdDir has lost its file ownership: every entry of the snapshot tree
// is root:root. Real image layers always carry non-root entries (nginx's uid
// 101, elasticsearch's 1000, /etc/shadow's gid 42), so an all-root tree is the
// signature of a copy or restore that did not preserve attributes. Containers
// that run as non-root then cannot write inside their own image and every pod
// fails the same way, across restarts and reboots, until the cache is rebuilt
// (BF-1092).
//
// inspected is false when the tree could not be judged: no snapshots yet, or
// unreadable without privileges that were not available. reowned is then
// meaningless.
func ImageCacheReowned(containerdDir string) (reowned, inspected bool) {
	snapshots := filepath.Join(containerdDir, overlaySnapshotsSubdir)
	reowned, inspected, err := snapshotTreeAllRoot(snapshots)
	if err == nil {
		return reowned, inspected
	}
	if !errors.Is(err, fs.ErrPermission) || os.Geteuid() == 0 {
		return false, false
	}
	// containerd keeps the snapshot tree 0700 root. Take a privileged look
	// only when it needs no password (sudo -n), so a diagnostic never prompts.
	return snapshotTreeAllRootWithSudo(snapshots)
}

// snapshotTreeAllRoot walks dir until the first entry not owned by root:root.
// Symlinks are not followed. Entries that cannot be read are skipped.
func snapshotTreeAllRoot(dir string) (allRoot, inspected bool, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, false, nil
		}
		return false, false, err
	}
	if len(entries) == 0 {
		return false, false, nil
	}
	judged, nonRoot := false, false
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		uid, gid, ok := ownerOf(info)
		if !ok {
			return filepath.SkipAll
		}
		judged = true
		if uid != 0 || gid != 0 {
			nonRoot = true
			return filepath.SkipAll
		}
		return nil
	})
	if !judged {
		return false, false, nil
	}
	return !nonRoot, true, nil
}

func snapshotTreeAllRootWithSudo(dir string) (allRoot, inspected bool) {
	any, err := exec.Command("sudo", "-n", "find", dir, "-mindepth", "1", "-maxdepth", "1", "-print", "-quit").Output()
	if err != nil || strings.TrimSpace(string(any)) == "" {
		return false, false
	}
	nonRoot, err := exec.Command("sudo", "-n", "find", dir, "-mindepth", "1",
		"(", "!", "-uid", "0", "-o", "!", "-gid", "0", ")", "-print", "-quit").Output()
	if err != nil {
		return false, false
	}
	return strings.TrimSpace(string(nonRoot)) == "", true
}

// RepairReownedImageCache rebuilds the containerd image cache when its
// ownership has been lost. Only valid while no cluster exists: the cache is a
// derivative of the local registry and is re-pulled by the install that
// follows, so nothing is lost.
func RepairReownedImageCache() error {
	dir := GetContainerdDataDir()
	reowned, inspected := ImageCacheReowned(dir)
	if !inspected || !reowned {
		return nil
	}
	log.Warnf("Container image cache %s has lost its file ownership (every file is root-owned); rebuilding it from the local registry", dir)
	if err := RemovePath(dir); err != nil {
		return fmt.Errorf("failed to remove image cache %s: %w", dir, err)
	}
	return EnsurePrivateDir(dir)
}

// WarnIfImageCacheReowned tells the operator how to recover an image cache
// whose ownership was lost, for commands that cannot rebuild it themselves
// because the cluster already exists.
func WarnIfImageCacheReowned() {
	dir := GetContainerdDataDir()
	if reowned, inspected := ImageCacheReowned(dir); inspected && reowned {
		log.Warnf("Container image cache %s has lost its file ownership; pods that run as non-root fail with 'permission denied' inside their own image. Run the reinstall command (leap server reinstall) to rebuild it; application data is kept.", dir)
	}
}
