package output

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Prepares and saves a path for tests covering the complete file-output operation.
func writeForTest(path string, content []byte) error {
	w := FileWriter{}
	destination, err := w.Prepare(path)
	if err != nil {
		return err
	}
	return w.Write(destination, content)
}

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
			require.NoError(t, writeForTest(path, []byte("new")))
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
	require.NoError(t, writeForTest(link, []byte("new")))
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
	require.Error(t, writeForTest(dir, []byte("new")))
	parent := filepath.Join(dir, "parent")
	require.NoError(t, os.WriteFile(parent, []byte("keep"), 0600))
	require.ErrorContains(t, writeForTest(filepath.Join(parent, "child"), nil), "not a directory")
	content, err := os.ReadFile(parent)
	require.NoError(t, err)
	assert.Equal(t, "keep", string(content))
	link := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink("missing", link))
	require.ErrorContains(t, writeForTest(link, nil), "failed to resolve output symlink")
	_, err = os.Readlink(link)
	require.NoError(t, err)
	files, err := filepath.Glob(filepath.Join(dir, ".dd-conf-gen-*"))
	require.NoError(t, err)
	assert.Empty(t, files)
}

// Destination preflight must reject invalid paths and leave missing directories uncreated.
func TestFileWriterPrepare(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "missing", "out.yaml")
	w := FileWriter{}
	destination, err := w.Prepare(path)
	require.NoError(t, err)
	assert.Equal(t, path, destination.Path())
	assert.NoDirExists(t, filepath.Dir(path))
	_, err = w.Prepare(dir)
	require.Error(t, err)
	file := filepath.Join(dir, "parent")
	require.NoError(t, os.WriteFile(file, nil, 0600))
	_, err = w.Prepare(filepath.Join(file, "child.yaml"))
	require.Error(t, err)
}

// Retargets the original alias after preparation and verifies saving remains bound to the validated destination.
func TestPreparedDestinationPinsResolvedPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	first := filepath.Join(dir, "first.yaml")
	second := filepath.Join(dir, "second.yaml")
	alias := filepath.Join(dir, "alias")
	require.NoError(t, os.WriteFile(first, []byte("first"), 0600))
	require.NoError(t, os.WriteFile(second, []byte("second"), 0600))
	require.NoError(t, os.Symlink(first, alias))
	w := FileWriter{}
	destination, err := w.Prepare(alias)
	require.NoError(t, err)
	require.NoError(t, os.Remove(alias))
	require.NoError(t, os.Symlink(second, alias))
	require.NoError(t, w.Write(destination, []byte("updated")))
	for path, expected := range map[string]string{first: "updated", second: "second"} {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, expected, string(data))
	}
}

// Changes the canonical destination after preparation and requires revalidation without altering the new target.
func TestPreparedDestinationRevalidation(t *testing.T) {
	t.Parallel()
	for _, change := range []string{"directory", "symlink", "ancestor symlink"} {
		t.Run(change, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "nested", "out.yaml")
			other := filepath.Join(dir, "other")
			require.NoError(t, os.Mkdir(other, 0700))
			otherFile := filepath.Join(other, "out.yaml")
			require.NoError(t, os.WriteFile(otherFile, []byte("keep"), 0600))
			w := FileWriter{}
			destination, err := w.Prepare(path)
			require.NoError(t, err)
			switch change {
			case "directory":
				require.NoError(t, os.MkdirAll(path, 0700))
			case "symlink":
				require.NoError(t, os.Mkdir(filepath.Dir(path), 0700))
				require.NoError(t, os.Symlink(otherFile, path))
			case "ancestor symlink":
				require.NoError(t, os.Symlink(other, filepath.Dir(path)))
			}
			require.Error(t, w.Write(destination, []byte("replacement")))
			data, err := os.ReadFile(otherFile)
			require.NoError(t, err)
			assert.Equal(t, "keep", string(data))
		})
	}
	require.Error(t, (FileWriter{}).Write(Destination{}, nil))
}
