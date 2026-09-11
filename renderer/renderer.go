package renderer

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"text/template"

	"github.com/moepig/dd-conf-gen/internal/logging"
	"github.com/moepig/dd-conf-gen/providers"
)

// TemplateData represents data passed to templates
type TemplateData struct {
	Resources []providers.Resource
}

// Renderer handles template rendering
type Renderer struct {
	templateDir string
}

// NewRenderer creates a new Renderer
func NewRenderer(templateDir string) *Renderer {
	return &Renderer{
		templateDir: templateDir,
	}
}

// Render renders a template with the given data
func (r *Renderer) Render(templatePath string, data TemplateData) ([]byte, error) {
	return r.RenderContext(context.Background(), templatePath, data)
}

// Renders a template file with data, sending diagnostics to the context logger.
func (r *Renderer) RenderContext(ctx context.Context, templatePath string, data TemplateData) ([]byte, error) {
	// Read template file
	content, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read template file: %w", err)
	}

	logging.FromContext(ctx).Debug("Read template file", "path", templatePath, "content", string(content))

	// Parse template
	tmpl, err := template.New("config").Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	logging.FromContext(ctx).Debug("Rendering template with data", "resources_count", len(data.Resources))

	// Execute template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.Bytes(), nil
}
