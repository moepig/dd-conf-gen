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

// Renders templates from the file paths supplied by callers.
type Renderer struct{}

// Creates a stateless template renderer.
func NewRenderer() *Renderer {
	return &Renderer{}
}

// Render renders a template with the given data
func (r *Renderer) Render(templatePath string, data TemplateData) ([]byte, error) {
	return r.RenderContext(context.Background(), templatePath, data)
}

// Renders a template file with data, sending diagnostics to the context logger.
func (r *Renderer) RenderContext(ctx context.Context, templatePath string, data TemplateData) ([]byte, error) {
	compiled, err := r.CompileContext(ctx, templatePath)
	if err != nil {
		return nil, err
	}
	return compiled.RenderContext(ctx, data)
}

// Holds a parsed template that can be rendered without rereading the source file.
type CompiledTemplate struct{ template *template.Template }

// Reads and parses a template file, returning errors before rendering or discovery is needed.
func (r *Renderer) CompileContext(ctx context.Context, templatePath string) (*CompiledTemplate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Read template file
	content, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read template file: %w", err)
	}

	logging.FromContext(ctx).Debug("Read template file", "path", templatePath, "bytes", len(content))

	// Parse template
	tmpl, err := template.New("config").Option("missingkey=error").Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &CompiledTemplate{template: tmpl}, nil
}

// Executes a compiled template with data and returns no partial content on failure.
func (t *CompiledTemplate) RenderContext(ctx context.Context, data TemplateData) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	logging.FromContext(ctx).Debug("Rendering template with data", "resources_count", len(data.Resources))

	// Execute template
	var buf bytes.Buffer
	if err := t.template.Execute(contextWriter{ctx: ctx, buffer: &buf}, data); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

type contextWriter struct {
	ctx    context.Context
	buffer *bytes.Buffer
}

// Appends template output unless cancellation has been requested, returning the cancellation error without writing.
func (w contextWriter) Write(data []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.buffer.Write(data)
}
