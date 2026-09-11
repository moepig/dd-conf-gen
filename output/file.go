package output

import (
	"fmt"
	"os"
	"path/filepath"
)

// Replaces each output through a temporary file in the destination directory.
type FileWriter struct{}

// Writes content to path, preserving existing file permissions and following existing symlinks.
// Failures before the rename leave the existing destination unchanged; dangling symlinks are rejected.
func (FileWriter) Write(path string, content []byte) error {
	target := path
	info, err := os.Lstat(path)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		target, err = filepath.EvalSymlinks(path)
		if err != nil {
			return fmt.Errorf("failed to resolve output symlink: %w", err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to inspect output file: %w", err)
	}
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory '%s': %w", dir, err)
	}
	mode := os.FileMode(0644)
	if info, err := os.Stat(target); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("output destination is not a regular file: %s", path)
		}
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to inspect output file: %w", err)
	}
	temp, err := os.CreateTemp(dir, ".dd-conf-gen-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary output file: %w", err)
	}
	defer os.Remove(temp.Name())
	defer temp.Close()
	if _, err := temp.Write(content); err != nil {
		return fmt.Errorf("failed to write temporary output file: %w", err)
	}
	if err := temp.Chmod(mode); err != nil {
		return fmt.Errorf("failed to set output permissions: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("failed to sync temporary output file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("failed to close temporary output file: %w", err)
	}
	if err := os.Rename(temp.Name(), target); err != nil {
		return fmt.Errorf("failed to replace output file: %w", err)
	}
	return nil
}
