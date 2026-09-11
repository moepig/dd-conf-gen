package output

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Creates and replaces real files to verify complete content, permission preservation, and temporary file cleanup.
func TestFileWriter(t *testing.T) {
	t.Parallel()
	for _, existing := range []bool{false, true} {
		name := "new"
		if existing {
			name = "existing"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), "nested")
			path := filepath.Join(dir, "redis.yaml")
			mode := os.FileMode(0644)
			if existing {
				require.NoError(t, os.Mkdir(dir, 0755))
				require.NoError(t, os.WriteFile(path, []byte("old configuration with trailing bytes"), 0600))
				mode = 0600
			}
			require.NoError(t, (FileWriter{}).Write(path, []byte("new")))
			content, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, "new", string(content))
			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, mode, info.Mode().Perm())
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			require.Len(t, entries, 1)
			assert.Equal(t, "redis.yaml", entries[0].Name())
		})
	}
}

// Writing through a symlink must update its target while retaining the symlink itself.
func TestFileWriterSymlink(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "target.yaml")
	link := filepath.Join(dir, "link.yaml")
	require.NoError(t, os.WriteFile(target, []byte("old"), 0600))
	require.NoError(t, os.Symlink("target.yaml", link))
	require.NoError(t, (FileWriter{}).Write(link, []byte("new")))
	content, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "new", string(content))
	linkTarget, err := os.Readlink(link)
	require.NoError(t, err)
	assert.Equal(t, "target.yaml", linkTarget)
}

// Invalid destinations must fail without replacing a directory, parent file, or dangling symlink.
func TestFileWriterInvalidDestination(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	w := FileWriter{}
	require.Error(t, w.Write(dir, []byte("new")))
	parent := filepath.Join(dir, "parent")
	require.NoError(t, os.WriteFile(parent, []byte("keep"), 0600))
	require.ErrorContains(t, w.Write(filepath.Join(parent, "child"), nil), "failed to")
	content, err := os.ReadFile(parent)
	require.NoError(t, err)
	assert.Equal(t, "keep", string(content))
	link := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink("missing", link))
	require.ErrorContains(t, w.Write(link, nil), "failed to resolve output symlink")
	_, err = os.Readlink(link)
	require.NoError(t, err)
	files, err := filepath.Glob(filepath.Join(dir, ".dd-conf-gen-*"))
	require.NoError(t, err)
	assert.Empty(t, files)
}
