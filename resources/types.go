package resources

// Resource represents a discovered cloud resource
type Resource struct {
	Host     string                 // Hostname or endpoint
	Port     int                    // Port number
	Tags     map[string]string      // Mapped Datadog tags
	Metadata map[string]interface{} // Type-specific additional data
}
