package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/moepig/dd-conf-gen/config"
	"github.com/moepig/dd-conf-gen/internal/logging"
	"github.com/moepig/dd-conf-gen/providers"
	"github.com/moepig/dd-conf-gen/renderer"
)

type application struct {
	registry *providers.Registry
	writer   outputWriter
}

// Saves one output, reporting failures to the caller.
type outputWriter interface {
	Write(path string, content []byte) error
}

type generatedOutput struct {
	path    string
	content []byte
}

// Generates every output, then saves files in configuration order, stopping at the first save failure.
func (app *application) run(ctx context.Context, configPath string) error {
	outputs, err := app.generate(ctx, configPath)
	if err != nil {
		return err
	}
	for _, output := range outputs {
		if err := app.writer.Write(output.path, output.content); err != nil {
			return fmt.Errorf("failed to write output file '%s': %w", output.path, err)
		}
		logging.FromContext(ctx).Info("Written output file", "path", output.path)
	}
	logging.FromContext(ctx).Info("Done!")
	return nil
}

// Generates all outputs before any destination is changed.
func (app *application) generate(ctx context.Context, configPath string) ([]generatedOutput, error) {
	// Load generation configuration
	logging.FromContext(ctx).Info("Loading generation configuration", "config_path", configPath)
	genCfg, err := config.LoadGenConfigContext(ctx, configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load generation config: %w", err)
	}
	requests, err := app.prepareResources(genCfg.Resources)
	if err != nil {
		return nil, err
	}

	// Discover resources for each resource config
	logging.FromContext(ctx).Info("Discovering resources")
	resourceMap := make(map[string][]providers.Resource)
	for _, request := range requests {
		logging.FromContext(ctx).Info("Discovering resource",
			"name", request.name,
			"type", request.provider.Type(),
			"region", request.config.Region)

		logging.FromContext(ctx).Debug("Provider config", "region", request.config.Region, "filters", request.config.Filters)

		discoveredResources, err := request.provider.Discover(ctx, request.config)
		if err != nil {
			return nil, fmt.Errorf("failed to discover resources for '%s': %w", request.name, err)
		}

		resourceMap[request.name] = discoveredResources
		logging.FromContext(ctx).Info("Found resources",
			"name", request.name,
			"count", len(discoveredResources))
		logging.FromContext(ctx).Debug("Resource details", "name", request.name, "resources", discoveredResources)
	}

	// Render templates and write output files
	logging.FromContext(ctx).Info("Generating output files")
	rend := renderer.NewRenderer()

	var outputs []generatedOutput
	for _, outCfg := range genCfg.Outputs {
		logging.FromContext(ctx).Info("Rendering template", "output_file", outCfg.OutputFile)

		// Get resources for this output
		discoveredResources, ok := resourceMap[outCfg.Data.ResourceName]
		if !ok {
			return nil, fmt.Errorf("resource '%s' not found for output '%s'", outCfg.Data.ResourceName, outCfg.OutputFile)
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
			return nil, fmt.Errorf("failed to render template for '%s': %w", outCfg.OutputFile, err)
		}

		logging.FromContext(ctx).Debug("Rendered output", "output_file", outCfg.OutputFile, "content", string(output))

		outputs = append(outputs, generatedOutput{path: outCfg.OutputFile, content: output})
	}
	return outputs, nil
}

type resourceRequest struct {
	name     string
	provider providers.Provider
	config   providers.ProviderConfig
}

// Resolves and validates every resource definition before any discovery begins.
func (app *application) prepareResources(resources []config.ResourceConfig) ([]resourceRequest, error) {
	requests := make([]resourceRequest, 0, len(resources))
	for _, resource := range resources {
		provider, err := app.registry.Get(resource.Type)
		if err != nil {
			return nil, fmt.Errorf("failed to get provider for resource '%s': %w", resource.Name, err)
		}
		cfg := providers.ProviderConfig{Region: resource.Region, Filters: resource.Filters}
		if err := provider.ValidateConfig(cfg); err != nil {
			return nil, fmt.Errorf("invalid provider config for resource '%s': %w", resource.Name, err)
		}
		requests = append(requests, resourceRequest{name: resource.Name, provider: provider, config: cfg})
	}
	return requests, nil
}
