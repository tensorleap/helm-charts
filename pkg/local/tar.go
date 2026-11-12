package local

import (
	"archive/tar"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

// DirSize is the total size of the regular files under root.
func DirSize(root string) (int64, error) {
	var size int64
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			size += info.Size()
		}
		return nil
	})
	return size, err
}

// TarDir writes the tree under root to w as a tar stream whose entries sit
// under prefix, so extracting the stream in a directory creates prefix there
// holding root's contents. Ownership is not kept (the consumer normalises the
// extracted tree); symlinks are kept, other special files skipped. progress,
// when not nil, receives the running total of file bytes written.
func TarDir(root, prefix string, w io.Writer, progress func(written int64)) error {
	return TarDirShard(root, prefix, w, 0, 1, progress)
}

// TarDirShard is TarDir for one of shards slices of the tree: every shard
// carries all the directories and every shards-th regular file (and symlink),
// so extracting all shards into one directory, concurrently, rebuilds the
// tree. The walk order is lexical, hence the same in every shard.
func TarDirShard(root, prefix string, w io.Writer, shard, shards int, progress func(written int64)) error {
	tw := tar.NewWriter(w)
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: prefix + "/", Mode: 0o777, Format: tar.FormatPAX}); err != nil {
		return err
	}
	var written, index int64
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil || rel == "." {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		link := ""
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			if link, err = os.Readlink(p); err != nil {
				return err
			}
		case !info.Mode().IsRegular() && !info.IsDir():
			return nil
		}
		if !info.IsDir() {
			index++
			if (index-1)%int64(shards) != int64(shard) {
				return nil
			}
		}
		hdr, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		hdr.Name = path.Join(prefix, filepath.ToSlash(rel))
		if info.IsDir() {
			hdr.Name += "/"
		}
		hdr.Format = tar.FormatPAX
		hdr.Uid, hdr.Gid, hdr.Uname, hdr.Gname = 0, 0, "", ""
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		n, err := io.CopyN(tw, f, hdr.Size)
		f.Close()
		written += n
		if err != nil {
			return fmt.Errorf("%s changed while being copied: %w", p, err)
		}
		if progress != nil {
			progress(written)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return tw.Close()
}
