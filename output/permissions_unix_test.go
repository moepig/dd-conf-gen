//go:build unix

package output

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Changes umask in isolated subprocesses and verifies new-file restrictions and exact existing-file modes.
func TestFileWriterUmask(t *testing.T) {
	if maskText := os.Getenv("DD_CONF_TEST_UMASK"); maskText != "" {
		mask, err := strconv.ParseInt(maskText, 8, 32)
		require.NoError(t, err)
		dir := t.TempDir()
		syscall.Umask(int(mask))
		path := filepath.Join(dir, "new.yaml")
		require.NoError(t, (FileWriter{}).Write(path, []byte("configuration")))
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0644)&^os.FileMode(mask), info.Mode().Perm())
		for _, mode := range []os.FileMode{0000, 0600, 0640, 0644} {
			require.NoError(t, os.Chmod(path, mode))
			require.NoError(t, (FileWriter{}).Write(path, []byte("replacement")))
			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, mode, info.Mode().Perm())
		}
		return
	}
	t.Parallel()
	for _, mask := range []string{"000", "022", "077", "777"} {
		t.Run(mask, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestFileWriterUmask$")
			cmd.Env = append(os.Environ(), "DD_CONF_TEST_UMASK="+mask)
			data, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", data)
		})
	}
}
