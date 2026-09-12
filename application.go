package main

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/moepig/dd-conf-gen/config"
	"github.com/moepig/dd-conf-gen/internal/logging"
	"github.com/moepig/dd-conf-gen/output"
	"github.com/moepig/dd-conf-gen/providers"
	"github.com/moepig/dd-conf-gen/renderer"
)

// Holds the provider registry and output writer for a generation run.
type application struct {
	registry *providers.Registry
	writer   outputWriter
}

// Prepares destinations and saves output contents. Failures are returned as errors.
type outputWriter interface {
	// Validates path without writing and returns a destination, or an error for an invalid path.
	Prepare(path string) (output.Destination, error)

	// Saves content at destination and returns an error on failure.
	Write(destination output.Destination, content []byte) error
}

// Holds a prepared destination and its complete rendered contents.
type generatedOutput struct {
	destination output.Destination
	content     []byte
}

// Generates and saves the outputs defined by configPath, using ctx for cancellation and logging.
//
// Returns the first generation, save, or cancellation error, or nil on success. Outputs are saved in configuration order; completed saves are not rolled back.
func (app *application) run(ctx context.Context, configPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Complete all generation before the first save to avoid writes on discovery or rendering errors.
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

// Builds output contents and destinations from configPath, using ctx for cancellation and logging.
//
// Returns all generated outputs in configuration order without saving files. Returns nil outputs and an error if loading, preparation, discovery, rendering, or cancellation fails.
func (app *application) generate(ctx context.Context, configPath string) ([]generatedOutput, error) {
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

	logging.FromContext(ctx).Info("Generating output files")
	var outputs []generatedOutput
	for _, prepared := range preparedOutputs {
		path := prepared.destination.Path()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		logging.FromContext(ctx).Info("Rendering template", "output_file", path)

		discoveredResources := selectResources(resourceMap, prepared.resourceNames)

		templateData := renderer.TemplateData{
			Resources: discoveredResources,
		}

		output, err := prepared.template.Render(ctx, templateData)
		if err != nil {
			return nil, fmt.Errorf("failed to render template for '%s': %w", path, err)
		}

		logging.FromContext(ctx).Debug("Rendered output", "output_file", path, "bytes", len(output))

		outputs = append(outputs, generatedOutput{destination: prepared.destination, content: output})
	}
	return outputs, nil
}

// Holds resource identity, region, and a prepared search.
type resourceRequest struct {
	name     string
	kind     string
	region   string
	discover providers.Discovery
}

// Prepares searches for the supplied resource definitions without executing them.
//
// Returns requests in definition order, or nil and an error for an unavailable provider, invalid settings, or a nil search.
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

// Holds a resource reference, validated destination, and compiled template.
type preparedOutput struct {
	resourceNames []string
	destination   output.Destination
	template      *renderer.CompiledTemplate
}

// Prepares templates and destinations for outputs without writing files.
//
// Resolves relative template paths against configDir and uses ctx for cancellation and logging. Returns prepared outputs in definition order, or nil and an error for cancellation, template errors, invalid destinations, or duplicate resolved paths.
func (app *application) prepareOutputs(ctx context.Context, outputs []config.OutputConfig, configDir string) ([]preparedOutput, error) {
	prepared := make([]preparedOutput, 0, len(outputs))
	for _, out := range outputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		path := out.Template
		if !filepath.IsAbs(path) {
			path = filepath.Join(configDir, path)
		}
		template, err := renderer.Compile(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("failed to render template for '%s': %w", out.OutputFile, err)
		}
		prepared = append(prepared, preparedOutput{resourceNames: out.Data.Names(), template: template})
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

// Combines named searches, deduplicating endpoints with the first definition taking precedence and sorting by host and port. A single search retains its original order and duplicates.
func selectResources(resourceMap map[string][]providers.Resource, names []string) []providers.Resource {
	if len(names) == 1 {
		return resourceMap[names[0]]
	}
	type endpoint struct {
		host string
		port int
	}
	seen := make(map[endpoint]bool)
	var result []providers.Resource
	for _, name := range names {
		for _, resource := range resourceMap[name] {
			key := endpoint{resource.Host, resource.Port}
			if !seen[key] {
				seen[key] = true
				result = append(result, resource)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Host == result[j].Host {
			return result[i].Port < result[j].Port
		}
		return result[i].Host < result[j].Host
	})
	return result
}
