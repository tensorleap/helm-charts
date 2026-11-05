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
	tw := tar.NewWriter(w)
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: prefix + "/", Mode: 0o777, Format: tar.FormatPAX}); err != nil {
		return err
	}
	var written int64
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
