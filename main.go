package main

import (
	"os"

	"github.com/moepig/dd-conf-gen/internal/app"
)

// Build version; release builds may override it through linker flags.
var version = "0.34.1"

// Initializes the application and exits with the CLI status.
func main() {
	os.Exit(app.Run(version, os.Args[1:], os.Stdout, os.Stderr))
}
