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

// Holds the resource list available to template expressions.
type TemplateData struct {
	Resources []providers.Resource
}

// Holds a parsed template that can be rendered without rereading the source file.
type CompiledTemplate struct{ template *template.Template }

// Reads and parses the template at templatePath, using ctx for cancellation and logging.
//
// Returns a compiled template configured to reject missing map keys accessed with dot notation, or nil and a read, parse, or cancellation error.
func Compile(ctx context.Context, templatePath string) (*CompiledTemplate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	content, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read template file: %w", err)
	}

	logging.FromContext(ctx).Debug("Read template file", "path", templatePath, "bytes", len(content))

	tmpl, err := template.New("config").Option("missingkey=error").Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &CompiledTemplate{template: tmpl}, nil
}

// Renders data with the compiled template, using ctx for cancellation and logging.
//
// Returns independently buffered output, or nil and an execution or cancellation error with no partial content.
func (t *CompiledTemplate) Render(ctx context.Context, data TemplateData) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	logging.FromContext(ctx).Debug("Rendering template with data", "resources_count", len(data.Resources))

	var buf bytes.Buffer
	if err := t.template.Execute(contextWriter{ctx: ctx, buffer: &buf}, data); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// Holds cancellation state and a buffer for appended bytes.
type contextWriter struct {
	ctx    context.Context
	buffer *bytes.Buffer
}

// Appends data to the buffer, returning the byte count and error.
//
// If the context is cancelled, returns zero and the context error without writing.
func (w contextWriter) Write(data []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.buffer.Write(data)
}
