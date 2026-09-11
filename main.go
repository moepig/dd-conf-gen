package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/moepig/dd-conf-gen/internal/logging"
	"github.com/moepig/dd-conf-gen/output"
	"github.com/moepig/dd-conf-gen/providers"
	"github.com/moepig/dd-conf-gen/providers/elasticache"
)

var version = "0.4.0"

func main() {
	registry := &providers.Registry{}
	registry.Register(elasticache.NewProvider())
	app := &application{registry: registry, writer: output.FileWriter{}}
	os.Exit(app.runCLI(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

// Parses args and runs generation, writing diagnostics to stderr and returning an exit code.
func (app *application) runCLI(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("dd-conf-gen", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "", "Path to generation configuration file")
	logLevelStr := flags.String("log-level", "info", "Log level (debug, info, warn, error)")
	showVersion := flags.Bool("version", false, "Print version and exit")
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
	var logLevel slog.Level
	switch *logLevelStr {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		fmt.Fprintf(stderr, "Error: invalid log level '%s' (must be debug, info, warn, or error)\n", *logLevelStr)
		flags.Usage()
		return 1
	}
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
