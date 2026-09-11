package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/moepig/dd-conf-gen/config"
	"github.com/moepig/dd-conf-gen/internal/logging"
	"github.com/moepig/dd-conf-gen/providers"
	"github.com/moepig/dd-conf-gen/renderer"
)

type application struct {
	registry *providers.Registry
}

func (app *application) run(ctx context.Context, configPath string) error {
	// Load generation configuration
	logging.FromContext(ctx).Info("Loading generation configuration", "config_path", configPath)
	genCfg, err := config.LoadGenConfigContext(ctx, configPath)
	if err != nil {
		return fmt.Errorf("failed to load generation config: %w", err)
	}

	// Discover resources for each resource config
	logging.FromContext(ctx).Info("Discovering resources")
	resourceMap := make(map[string][]providers.Resource)
	for _, resCfg := range genCfg.Resources {
		logging.FromContext(ctx).Info("Discovering resource",
			"name", resCfg.Name,
			"type", resCfg.Type,
			"region", resCfg.Region)

		provider, err := app.registry.Get(resCfg.Type)
		if err != nil {
			return fmt.Errorf("failed to get provider for resource '%s': %w", resCfg.Name, err)
		}

		providerCfg := providers.ProviderConfig{
			Region:  resCfg.Region,
			Filters: resCfg.Filters,
		}

		logging.FromContext(ctx).Debug("Provider config", "region", providerCfg.Region, "filters", providerCfg.Filters)

		discoveredResources, err := provider.Discover(ctx, providerCfg)
		if err != nil {
			return fmt.Errorf("failed to discover resources for '%s': %w", resCfg.Name, err)
		}

		resourceMap[resCfg.Name] = discoveredResources
		logging.FromContext(ctx).Info("Found resources",
			"name", resCfg.Name,
			"count", len(discoveredResources))
		logging.FromContext(ctx).Debug("Resource details", "name", resCfg.Name, "resources", discoveredResources)
	}

	// Render templates and write output files
	logging.FromContext(ctx).Info("Generating output files")
	rend := renderer.NewRenderer("")

	for _, outCfg := range genCfg.Outputs {
		logging.FromContext(ctx).Info("Rendering template", "output_file", outCfg.OutputFile)

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
		output, err := rend.RenderContext(ctx, templatePath, templateData)
		if err != nil {
			return fmt.Errorf("failed to render template for '%s': %w", outCfg.OutputFile, err)
		}

		logging.FromContext(ctx).Debug("Rendered output", "output_file", outCfg.OutputFile, "content", string(output))

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

		logging.FromContext(ctx).Info("Written output file", "path", outCfg.OutputFile)
	}

	logging.FromContext(ctx).Info("Done!")
	return nil
}
