package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/moepig/dd-conf-gen/config"
	"github.com/moepig/dd-conf-gen/renderer"
	"github.com/moepig/dd-conf-gen/resources"
	"github.com/moepig/dd-conf-gen/resources/elasticache"
)

func init() {
	// Register providers
	resources.Register(elasticache.NewProvider())
}

func main() {
	// Command line arguments
	configPath := flag.String("config", "", "Path to generation configuration file")
	flag.Parse()

	// Validate config option
	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "Error: -config option is required")
		flag.Usage()
		os.Exit(1)
	}

	ctx := context.Background()

	// Run the application
	if err := run(ctx, *configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, configPath string) error {
	// Load generation configuration
	fmt.Fprintf(os.Stderr, "Loading generation configuration from %s...\n", configPath)
	genCfg, err := config.LoadGenConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load generation config: %w", err)
	}

	// Discover resources for each resource config
	fmt.Fprintf(os.Stderr, "Discovering resources...\n")
	resourceMap := make(map[string][]resources.Resource)
	for _, resCfg := range genCfg.Resources {
		fmt.Fprintf(os.Stderr, "  - Discovering %s (type: %s, region: %s)...\n", resCfg.Name, resCfg.Type, resCfg.Region)

		provider, err := resources.Get(resCfg.Type)
		if err != nil {
			return fmt.Errorf("failed to get provider for resource '%s': %w", resCfg.Name, err)
		}

		providerCfg := resources.ProviderConfig{
			Region:  resCfg.Region,
			Filters: resCfg.Filters,
		}

		discoveredResources, err := provider.Discover(ctx, providerCfg)
		if err != nil {
			return fmt.Errorf("failed to discover resources for '%s': %w", resCfg.Name, err)
		}

		resourceMap[resCfg.Name] = discoveredResources
		fmt.Fprintf(os.Stderr, "    Found %d resource(s)\n", len(discoveredResources))
	}

	// Render templates and write output files
	fmt.Fprintf(os.Stderr, "Generating output files...\n")
	rend := renderer.NewRenderer("")

	for _, outCfg := range genCfg.Outputs {
		fmt.Fprintf(os.Stderr, "  - Rendering %s...\n", outCfg.OutputFile)

		// Get resources for this output
		discoveredResources, ok := resourceMap[outCfg.Data.ResourceName]
		if !ok {
			return fmt.Errorf("resource '%s' not found for output '%s'", outCfg.Data.ResourceName, outCfg.OutputFile)
		}

		// Prepare template data
		templateData := renderer.TemplateData{
			Resources: discoveredResources,
		}

		// Resolve template path (relative to generation config file)
		templatePath := outCfg.Template
		if !filepath.IsAbs(templatePath) {
			configDir := filepath.Dir(configPath)
			templatePath = filepath.Join(configDir, templatePath)
		}

		// Render template
		output, err := rend.Render(templatePath, templateData)
		if err != nil {
			return fmt.Errorf("failed to render template for '%s': %w", outCfg.OutputFile, err)
		}

		// Create output directory if needed
		outDir := filepath.Dir(outCfg.OutputFile)
		if outDir != "" && outDir != "." {
			if err := os.MkdirAll(outDir, 0755); err != nil {
				return fmt.Errorf("failed to create output directory '%s': %w", outDir, err)
			}
		}

		// Write output file
		if err := os.WriteFile(outCfg.OutputFile, output, 0644); err != nil {
			return fmt.Errorf("failed to write output file '%s': %w", outCfg.OutputFile, err)
		}

		fmt.Fprintf(os.Stderr, "    Written to %s\n", outCfg.OutputFile)
	}

	fmt.Fprintf(os.Stderr, "Done!\n")
	return nil
}
