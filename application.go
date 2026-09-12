package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/moepig/dd-conf-gen/config"
	"github.com/moepig/dd-conf-gen/internal/logging"
	"github.com/moepig/dd-conf-gen/output"
	"github.com/moepig/dd-conf-gen/providers"
	"github.com/moepig/dd-conf-gen/renderer"
)

type application struct {
	registry *providers.Registry
	writer   outputWriter
}

// Saves one output, reporting failures to the caller.
type outputWriter interface {
	Prepare(path string) (output.Destination, error)
	Write(destination output.Destination, content []byte) error
}

type generatedOutput struct {
	destination output.Destination
	content     []byte
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
		if err := app.writer.Write(output.destination, output.content); err != nil {
			return fmt.Errorf("failed to write output file '%s': %w", output.destination.Path(), err)
		}
		logging.FromContext(ctx).Info("Written output file", "path", output.destination.Path())
	}
	if err := ctx.Err(); err != nil {
		return err
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
			"type", request.kind,
			"region", request.region)

		discoveredResources, err := request.discover(ctx)
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
		path := prepared.destination.Path()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		logging.FromContext(ctx).Info("Rendering template", "output_file", path)

		// Get resources for this output
		discoveredResources, ok := resourceMap[prepared.resourceName]
		if !ok {
			return nil, fmt.Errorf("resource '%s' not found for output '%s'", prepared.resourceName, path)
		}

		// Prepare template data
		templateData := renderer.TemplateData{
			Resources: discoveredResources,
		}

		// Render template
		output, err := prepared.template.RenderContext(ctx, templateData)
		if err != nil {
			return nil, fmt.Errorf("failed to render template for '%s': %w", path, err)
		}

		logging.FromContext(ctx).Debug("Rendered output", "output_file", path, "bytes", len(output))

		outputs = append(outputs, generatedOutput{destination: prepared.destination, content: output})
	}
	return outputs, nil
}

type resourceRequest struct {
	name     string
	kind     string
	region   string
	discover providers.Discovery
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
		discover, err := provider.Prepare(cfg)
		if err != nil {
			return nil, fmt.Errorf("invalid provider config for resource '%s': %w", resource.Name, err)
		}
		if discover == nil {
			return nil, fmt.Errorf("provider returned no prepared discovery for resource '%s'", resource.Name)
		}
		requests = append(requests, resourceRequest{name: resource.Name, kind: resource.Type, region: resource.Region, discover: discover})
	}
	return requests, nil
}

type preparedOutput struct {
	resourceName string
	destination  output.Destination
	template     *renderer.CompiledTemplate
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
		prepared = append(prepared, preparedOutput{resourceName: out.Data.ResourceName, template: template})
	}
	paths := make(map[string]int, len(outputs))
	for i, out := range outputs {
		destination, err := app.writer.Prepare(out.OutputFile)
		if err != nil {
			return nil, fmt.Errorf("invalid output file '%s': %w", out.OutputFile, err)
		}
		if destination.Path() == "" {
			return nil, fmt.Errorf("empty prepared output destination: %s", out.OutputFile)
		}
		if previous, ok := paths[destination.Path()]; ok {
			return nil, fmt.Errorf("output[%d]: duplicate output_file with output[%d]: %s", i, previous, out.OutputFile)
		}
		paths[destination.Path()] = i
		prepared[i].destination = destination
	}
	return prepared, nil
}
