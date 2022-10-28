package flatfs

import (
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// rename behaves like os.Rename but can rename files across directories.
func rename(oldpath, newpath string) error {
	err := MoveFile(oldpath, newpath)
	if le, ok := err.(*os.LinkError); !ok || le.Err != os.ErrInvalid {
		return err
	}
	if filepath.Dir(oldpath) == filepath.Dir(newpath) {
		// We should not get here, but just in case
		// os.ErrInvalid is used for something else in the future.
		return err
	}

	src, err := os.Open(oldpath)
	if err != nil {
		return &os.LinkError{"rename", oldpath, newpath, err}
	}
	defer src.Close()

	fi, err := src.Stat()
	if err != nil {
		return &os.LinkError{"rename", oldpath, newpath, err}
	}
	if fi.Mode().IsDir() {
		return &os.LinkError{"rename", oldpath, newpath, syscall.EISDIR}
	}

	dst, err := os.OpenFile(newpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode())
	if err != nil {
		return &os.LinkError{"rename", oldpath, newpath, err}
	}

	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Remove(newpath)
		return &os.LinkError{"rename", oldpath, newpath, err}
	}
	if err := dst.Close(); err != nil {
		os.Remove(newpath)
		return &os.LinkError{"rename", oldpath, newpath, err}
	}

	// Copy mtime and mode from original file.
	// We need only one syscall if we avoid os.Chmod and os.Chtimes.
	dir := fi.Sys().(*syscall.Dir)
	var d syscall.Dir
	d.Null()
	d.Mtime = dir.Mtime
	d.Mode = dir.Mode
	_ = dirwstat(newpath, &d) // ignore error, as per mv(1)

	if err := os.Remove(oldpath); err != nil {
		return &os.LinkError{"rename", oldpath, newpath, err}
	}
	return nil
}

func MoveFile(sourcePath, destPath string) error {
	inputFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("couldn't open source file: %s", err)
	}
	outputFile, err := os.Create(destPath)
	if err != nil {
		inputFile.Close()
		return fmt.Errorf("couldn't open dest file: %s", err)
	}
	defer outputFile.Close()
	_, err = io.Copy(outputFile, inputFile)
	inputFile.Close()
	if err != nil {
		return fmt.Errorf("writing to output file failed: %s", err)
	}
	// The copy was successful, so now delete the original file
	err = os.Remove(sourcePath)
	if err != nil {
		return fmt.Errorf("failed removing original file: %s", err)
	}
	return nil
}

func dirwstat(name string, d *syscall.Dir) error {
	var buf [syscall.STATFIXLEN]byte

	n, err := d.Marshal(buf[:])
	if err != nil {
		return &os.PathError{"dirwstat", name, err}
	}
	if err = syscall.Wstat(name, buf[:n]); err != nil {
		return &os.PathError{"dirwstat", name, err}
	}
	return nil
}
