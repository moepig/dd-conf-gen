package renderer

import (
	"bytes"
	"fmt"
	"os"
	"text/template"

	"github.com/moepig/dd-conf-gen/resources"
)

// TemplateData represents data passed to templates
type TemplateData struct {
	Resources []resources.Resource
	Static    map[string]interface{}
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
	// Read template file
	content, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read template file: %w", err)
	}

	// Parse template
	tmpl, err := template.New("config").Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	// Execute template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.Bytes(), nil
}
