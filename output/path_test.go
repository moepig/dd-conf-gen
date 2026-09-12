package output

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Resolves aliases and missing ancestors, then verifies writes reach the same canonical destination.
func TestResolvePath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "b", "child"), 0700))
	require.NoError(t, os.Symlink("b/child", filepath.Join(dir, "link")))
	for name, suffix := range map[string]string{
		"parent after symlink":   "/link/../out.yaml",
		"missing before symlink": "/missing/../link/../out.yaml",
		"missing after symlink":  "/link/../new/../out.yaml",
	} {
		t.Run(name, func(t *testing.T) {
			path := dir + suffix
			resolved, err := ResolvePath(path)
			require.NoError(t, err)
			expected, err := filepath.EvalSymlinks(filepath.Join(dir, "b"))
			require.NoError(t, err)
			assert.Equal(t, filepath.Join(expected, "out.yaml"), resolved)
			require.NoError(t, writeForTest(path, []byte(name)))
			data, err := os.ReadFile(resolved)
			require.NoError(t, err)
			assert.Equal(t, name, string(data))
		})
	}
	require.NoError(t, os.Symlink("absent", filepath.Join(dir, "dangling")))
	require.NoError(t, os.Symlink("cycle", filepath.Join(dir, "cycle")))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file"), nil, 0600))
	for _, suffix := range []string{"/dangling/out", "/cycle/out", "/file/../out"} {
		_, err := ResolvePath(dir + suffix)
		require.Error(t, err)
	}
}
