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
	Validate(path string) error
	Write(path string, content []byte) error
}

type generatedOutput struct {
	path    string
	content []byte
}

// Generates every output, then saves files in configuration order, stopping at the first save failure.
func (app *application) run(ctx context.Context, configPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	outputs, err := app.generate(ctx, configPath)
	if err != nil {
		return err
	}
	for _, output := range outputs {
		if err := ctx.Err(); err != nil {
			return err
		}
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
	preparedOutputs, err := app.prepareOutputs(ctx, genCfg.Outputs, filepath.Dir(configPath))
	if err != nil {
		return nil, err
	}

	// Discover resources for each resource config
	logging.FromContext(ctx).Info("Discovering resources")
	resourceMap := make(map[string][]providers.Resource)
	for _, request := range requests {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		logging.FromContext(ctx).Info("Discovering resource",
			"name", request.name,
			"type", request.provider.Type(),
			"region", request.config.Region)

		discoveredResources, err := request.provider.Discover(ctx, request.config)
		if err != nil {
			return nil, fmt.Errorf("failed to discover resources for '%s': %w", request.name, err)
		}

		resourceMap[request.name] = discoveredResources
		logging.FromContext(ctx).Info("Found resources",
			"name", request.name,
			"count", len(discoveredResources))
	}

	// Render templates and write output files
	logging.FromContext(ctx).Info("Generating output files")
	var outputs []generatedOutput
	for _, prepared := range preparedOutputs {
		outCfg := prepared.config
		if err := ctx.Err(); err != nil {
			return nil, err
		}
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

		// Render template
		output, err := prepared.template.RenderContext(ctx, templateData)
		if err != nil {
			return nil, fmt.Errorf("failed to render template for '%s': %w", outCfg.OutputFile, err)
		}

		logging.FromContext(ctx).Debug("Rendered output", "output_file", outCfg.OutputFile, "bytes", len(output))

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

type preparedOutput struct {
	config   config.OutputConfig
	template *renderer.CompiledTemplate
}

// Parses all templates and validates destinations before resource discovery, without writing outputs.
func (app *application) prepareOutputs(ctx context.Context, outputs []config.OutputConfig, configDir string) ([]preparedOutput, error) {
	rend := renderer.NewRenderer()
	prepared := make([]preparedOutput, 0, len(outputs))
	for _, out := range outputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		path := out.Template
		if !filepath.IsAbs(path) {
			path = filepath.Join(configDir, path)
		}
		template, err := rend.CompileContext(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("failed to render template for '%s': %w", out.OutputFile, err)
		}
		prepared = append(prepared, preparedOutput{config: out, template: template})
	}
	for _, out := range outputs {
		if err := app.writer.Validate(out.OutputFile); err != nil {
			return nil, fmt.Errorf("invalid output file '%s': %w", out.OutputFile, err)
		}
	}
	return prepared, nil
}
