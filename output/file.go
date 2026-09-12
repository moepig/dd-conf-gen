package output

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
)

// Replaces each output through a temporary file in the destination directory.
type FileWriter struct{}

// Writes content to path, preserving existing file permissions and following existing symlinks.
// Failures before the rename leave the existing destination unchanged; dangling symlinks are rejected.
func (FileWriter) Write(path string, content []byte) error {
	target, mode, err := inspectDestination(path)
	if err != nil {
		return err
	}
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory '%s': %w", dir, err)
	}
	return writeTemporaryFile(target, content, mode)
}

// Checks existing destination types and symlinks without modifying the filesystem.
// Permissions, free space, and subsequent filesystem changes are checked by the actual write.
func (FileWriter) Validate(path string) error {
	_, _, err := inspectDestination(path)
	return err
}

// Resolves the destination and existing permissions, rejecting unusable destination types.
func inspectDestination(path string) (string, *os.FileMode, error) {
	target, err := ResolvePath(path)
	if err != nil {
		return "", nil, err
	}
	var mode *os.FileMode
	if info, err := os.Stat(target); err == nil {
		if !info.Mode().IsRegular() {
			return "", nil, fmt.Errorf("output destination is not a regular file: %s", path)
		}
		permissions := info.Mode().Perm()
		mode = &permissions
	} else if !os.IsNotExist(err) {
		return "", nil, fmt.Errorf("failed to inspect output file: %w", err)
	}
	return target, mode, nil
}

// Writes and replaces a destination using a temporary file in the same directory.
func writeTemporaryFile(target string, content []byte, mode *os.FileMode) error {
	dir := filepath.Dir(target)
	createMode := os.FileMode(0644)
	if mode != nil {
		createMode = 0600
	}
	temp, err := os.OpenFile(filepath.Join(dir, ".dd-conf-gen-"+rand.Text()), os.O_RDWR|os.O_CREATE|os.O_EXCL, createMode)
	if err != nil {
		return fmt.Errorf("failed to create temporary output file: %w", err)
	}
	defer os.Remove(temp.Name())
	defer temp.Close()
	if _, err := temp.Write(content); err != nil {
		return fmt.Errorf("failed to write temporary output file: %w", err)
	}
	if mode != nil {
		if err := temp.Chmod(*mode); err != nil {
			return fmt.Errorf("failed to set output permissions: %w", err)
		}
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
