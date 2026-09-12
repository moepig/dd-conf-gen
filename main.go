package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/moepig/dd-conf-gen/internal/logging"
	"github.com/moepig/dd-conf-gen/output"
	"github.com/moepig/dd-conf-gen/providers"
	"github.com/moepig/dd-conf-gen/providers/elasticache"
)

var version = "0.25.0"

func main() {
	registry := providers.NewRegistry(map[string]providers.Factory{
		"elasticache_redis": func() providers.Provider { return elasticache.NewProvider() },
	})
	app := &application{registry: registry, writer: output.FileWriter{}}
	os.Exit(app.runWithSignals(os.Args[1:], os.Stdout, os.Stderr))
}

// Runs the CLI with cancellation on interrupt or termination, releasing signal handlers on return.
func (app *application) runWithSignals(args []string, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return app.runCLI(ctx, args, stdout, stderr)
}

// Parses args and runs generation, writing diagnostics to stderr and returning an exit code.
func (app *application) runCLI(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("dd-conf-gen", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "", "Path to generation configuration file")
	var logLevel slog.Level
	flags.TextVar(&logLevel, "log-level", slog.LevelInfo, "Log level (debug, info, warn, error; case-insensitive, optional numeric offset)")
	showVersion := flags.Bool("version", false, "Print version and exit")
	timeout := flags.Duration("timeout", 5*time.Minute, "Execution timeout (positive duration, e.g. 30s or 5m)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, version)
		return 0
	}
	if *timeout <= 0 {
		fmt.Fprintln(stderr, "Error: -timeout must be positive")
		return 1
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: logLevel}))
	ctx = logging.WithLogger(ctx, logger)
	if *configPath == "" {
		fmt.Fprintln(stderr, "Error: -config option is required")
		flags.Usage()
		return 1
	}
	if err := app.run(ctx, *configPath); err != nil {
		logger.Error("Application failed", "error", err)
		return 1
	}
	return 0
}
