package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/moepig/dd-conf-gen/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Writes a valid configuration whose output must remain absent if discovery is cancelled.
func cancellationConfig(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	output := filepath.Join(dir, "out.yaml")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "redis.tmpl"), []byte("instances: []"), 0600))
	path := writeRunConfig(t, dir, config.GenConfig{
		Resources: []config.ResourceConfig{{Name: "redis", Type: "elasticache_redis", Region: "us-east-1"}},
		Outputs:   []config.OutputConfig{{Template: "redis.tmpl", OutputFile: output, Data: config.OutputData{ResourceName: "redis"}}},
	})
	return path, output
}

// A waiting mock provider must receive the CLI deadline, and no output may be saved after it expires.
func TestCLITimeout(t *testing.T) {
	t.Parallel()
	app, p := newTestApplication(t)
	p.On("ValidateConfig", mock.Anything).Return(nil).Once()
	p.On("Discover", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		ctx := args.Get(0).(context.Context)
		_, ok := ctx.Deadline()
		assert.True(t, ok)
		<-ctx.Done()
	}).Return(nil, context.DeadlineExceeded).Once()
	path, output := cancellationConfig(t)
	var stdout, stderr bytes.Buffer
	code := app.runCLI(context.Background(), []string{"-config", path, "-timeout=50ms"}, &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "context deadline exceeded")
	assert.NoFileExists(t, output)
}

// Cancellation must prevent saving even if a provider returns a successful result after cancellation.
func TestApplicationCancellationBeforeSave(t *testing.T) {
	t.Parallel()
	app, p := newTestApplication(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.On("ValidateConfig", mock.Anything).Return(nil).Once()
	p.On("Discover", mock.Anything, mock.Anything).Run(func(mock.Arguments) { cancel() }).Return(nil, nil).Once()
	path, output := cancellationConfig(t)
	require.ErrorIs(t, app.run(ctx, path), context.Canceled)
	assert.NoFileExists(t, output)
}

// Delivers actual signals to a child process with mocked discovery and verifies cancellation without output writes.
func TestCLISignals(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	for _, sig := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			path, output := cancellationConfig(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, executable, "-test.run=^TestSignalCLIProcess$")
			cmd.Env = append(os.Environ(), "DD_CONF_GEN_SIGNAL_TEST=1", "DD_CONF_GEN_SIGNAL_CONFIG="+path)
			stdout, err := cmd.StdoutPipe()
			require.NoError(t, err)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			require.NoError(t, cmd.Start())
			t.Cleanup(func() {
				if cmd.ProcessState == nil {
					cmd.Process.Kill()
					cmd.Wait()
				}
			})
			line, err := bufio.NewReader(stdout).ReadString('\n')
			require.NoError(t, err)
			require.Equal(t, "ready\n", line)
			require.NoError(t, cmd.Process.Signal(sig))
			err = cmd.Wait()
			var exitErr *exec.ExitError
			require.ErrorAs(t, err, &exitErr)
			assert.Equal(t, 1, exitErr.ExitCode())
			assert.Contains(t, stderr.String(), "context canceled")
			assert.NoFileExists(t, output)
		})
	}
}

// Runs signal-aware CLI execution only in the child process used by TestCLISignals.
func TestSignalCLIProcess(t *testing.T) {
	if os.Getenv("DD_CONF_GEN_SIGNAL_TEST") != "1" {
		return
	}
	app, p := newTestApplication(t)
	p.On("ValidateConfig", mock.Anything).Return(nil).Once()
	p.On("Discover", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		fmt.Fprintln(os.Stdout, "ready")
		<-args.Get(0).(context.Context).Done()
	}).Return(nil, context.Canceled).Once()
	os.Exit(app.runWithSignals([]string{"-config", os.Getenv("DD_CONF_GEN_SIGNAL_CONFIG")}, os.Stdout, os.Stderr))
}
