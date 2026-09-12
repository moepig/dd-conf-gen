package output

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Resolves path to an absolute path.
//
// Relative paths use the working directory. Existing symlinks are resolved before subsequent parent components, and missing components are allowed. Returns an error for an empty path, a non-directory ancestor, an inspection failure, or an unresolvable symlink.
func ResolvePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("output path is empty")
	}
	path = filepath.FromSlash(path)
	if !filepath.IsAbs(path) {
		dir, err := os.Getwd()
		if err != nil {
			return "", err
		}
		path = dir + string(filepath.Separator) + path
	}
	volume := filepath.VolumeName(path)
	current := volume + string(filepath.Separator)
	for _, component := range strings.Split(path[len(volume):], string(filepath.Separator)) {
		if component == "" {
			continue
		}
		if info, err := os.Stat(current); err == nil && !info.IsDir() {
			return "", fmt.Errorf("output ancestor is not a directory: %s", current)
		} else if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("failed to inspect output ancestor: %w", err)
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("failed to inspect output path: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			current, err = filepath.EvalSymlinks(current)
			if err != nil {
				return "", fmt.Errorf("failed to resolve output symlink: %w", err)
			}
		}
	}
	return current, nil
}
