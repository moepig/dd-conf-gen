package output

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
)

// Replaces each output through a temporary file in the destination directory.
type FileWriter struct{}

// Holds a validated absolute destination; its zero value cannot be written.
type Destination struct{ path string }

// Returns the resolved destination path used for duplicate detection and saving.
func (d Destination) Path() string { return d.path }

// Resolves and validates a destination without creating files or directories.
func (FileWriter) Prepare(path string) (Destination, error) {
	target, _, err := inspectDestination(path)
	if err != nil {
		return Destination{}, err
	}
	return Destination{path: target}, nil
}

// Replaces a prepared destination, rejecting changed symlink targets and rechecking permissions and file types.
// Failures before the rename leave the existing destination unchanged.
func (FileWriter) Write(destination Destination, content []byte) error {
	target, mode, err := inspectDestination(destination.path)
	if err != nil {
		return err
	}
	if target != destination.path {
		return fmt.Errorf("output destination changed after preparation: %s", destination.path)
	}
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory '%s': %w", dir, err)
	}
	return writeTemporaryFile(target, content, mode)
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
