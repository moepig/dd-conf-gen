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

// Creates a configuration and template in a temporary directory owned by t. Returns the configuration path and the absent output path, failing the test on write errors.
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

// Waits for the CLI deadline in mocked discovery and requires a failure exit code, a deadline diagnostic, and no output file.
func TestCLITimeout(t *testing.T) {
	t.Parallel()
	app, p := newTestApplication(t)
	p.On("Prepare", mock.Anything).Return(nil).Once()
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

// Cancels during mocked discovery that returns success and requires a cancellation error with no output file.
func TestApplicationCancellationBeforeSave(t *testing.T) {
	t.Parallel()
	app, p := newTestApplication(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.On("Prepare", mock.Anything).Return(nil).Once()
	p.On("Discover", mock.Anything, mock.Anything).Run(func(mock.Arguments) { cancel() }).Return(nil, nil).Once()
	path, output := cancellationConfig(t)
	require.ErrorIs(t, app.run(ctx, path), context.Canceled)
	assert.NoFileExists(t, output)
}

// Cancels inside the final mocked save and requires cancellation rather than reporting overall success.
func TestApplicationCancellationDuringFinalSave(t *testing.T) {
	t.Parallel()
	app, p := newTestApplication(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.On("Prepare", mock.Anything).Return(nil).Once()
	p.On("Discover", mock.Anything, mock.Anything).Return(nil, nil).Once()
	path, destination := cancellationConfig(t)
	writer := new(mockOutputWriter)
	writer.On("Prepare", destination).Return(nil).Once()
	writer.On("Write", destination, []byte("instances: []")).Run(func(mock.Arguments) { cancel() }).Return(nil).Once()
	app.writer = writer
	require.ErrorIs(t, app.run(ctx, path), context.Canceled)
	writer.AssertExpectations(t)
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

// Runs the CLI in a subprocess selected by an environment variable, with mocked discovery waiting for cancellation and returning the cancellation error.
func TestSignalCLIProcess(t *testing.T) {
	if os.Getenv("DD_CONF_GEN_SIGNAL_TEST") != "1" {
		return
	}
	app, p := newTestApplication(t)
	p.On("Prepare", mock.Anything).Return(nil).Once()
	p.On("Discover", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		fmt.Fprintln(os.Stdout, "ready")
		<-args.Get(0).(context.Context).Done()
	}).Return(nil, context.Canceled).Once()
	os.Exit(app.runWithSignals([]string{"-config", os.Getenv("DD_CONF_GEN_SIGNAL_CONFIG")}, os.Stdout, os.Stderr))
}
